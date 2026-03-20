using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Net.WebSockets;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace Buildermark;

public enum ServerStatus
{
    Stopped,
    Starting,
    Running,
    Error
}

public sealed class ServerManager : INotifyPropertyChanged, IDisposable
{
    public const int Port = 55022;

    private Process? _process;
    private Timer? _healthCheckTimer;
    private readonly HttpClient _httpClient = new() { Timeout = TimeSpan.FromSeconds(5) };
    private readonly nint _jobHandle;
    private ServerStatus _status = ServerStatus.Stopped;
    private string _errorMessage = "";
    private string _lastStderr = "";
    private CancellationTokenSource? _notifyWsCts;
    private ClientWebSocket? _activeWs;
    private int _notifyReconnectDelayMs = 1000;

    /// <summary>Raised when a notification arrives from the server.</summary>
    public event Action<string, string, string?>? NotificationReceived;

    public ServerManager()
    {
        _jobHandle = CreateJobObject(IntPtr.Zero, null);
        if (_jobHandle != IntPtr.Zero)
        {
            var info = new JOBOBJECT_EXTENDED_LIMIT_INFORMATION
            {
                BasicLimitInformation = new JOBOBJECT_BASIC_LIMIT_INFORMATION
                {
                    LimitFlags = 0x2000 // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
                }
            };
            int length = Marshal.SizeOf(info);
            nint ptr = Marshal.AllocHGlobal(length);
            try
            {
                Marshal.StructureToPtr(info, ptr, false);
                SetInformationJobObject(_jobHandle, 9 /* JobObjectExtendedLimitInformation */, ptr, (uint)length);
            }
            finally
            {
                Marshal.FreeHGlobal(ptr);
            }
        }
    }

    public event PropertyChangedEventHandler? PropertyChanged;

    public ServerStatus Status
    {
        get => _status;
        private set { _status = value; OnPropertyChanged(); OnPropertyChanged(nameof(StatusText)); }
    }

    public string ErrorMessage
    {
        get => _errorMessage;
        private set { _errorMessage = value; OnPropertyChanged(); OnPropertyChanged(nameof(StatusText)); }
    }

    public string StatusText => Status switch
    {
        ServerStatus.Stopped => "Server Stopped",
        ServerStatus.Starting => "Server Starting...",
        ServerStatus.Running => "Server Running",
        ServerStatus.Error => $"Error: {ErrorMessage}",
        _ => "Unknown"
    };

    public void Start()
    {
        if (_process is { HasExited: false })
            return;

        Status = ServerStatus.Starting;
        _lastStderr = "";

        var binaryPath = ResolveServerBinary();
        if (binaryPath == null)
        {
            Status = ServerStatus.Error;
            ErrorMessage = "Server binary not found";
            return;
        }

        var dbPath = ResolveDBPath();

        // Ensure the database directory exists.
        var dbDir = Path.GetDirectoryName(dbPath);
        if (dbDir != null && !Directory.Exists(dbDir))
            Directory.CreateDirectory(dbDir);

        var startInfo = new ProcessStartInfo
        {
            FileName = binaryPath,
            Arguments = $"-addr 127.0.0.1:{Port} -db \"{dbPath}\"",
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };

        try
        {
            _process = new Process { StartInfo = startInfo, EnableRaisingEvents = true };
            _process.OutputDataReceived += (_, e) =>
            {
                if (e.Data != null)
                    Trace.WriteLine($"[server stdout] {e.Data}");
            };
            _process.ErrorDataReceived += (_, e) =>
            {
                if (e.Data != null)
                {
                    Trace.WriteLine($"[server stderr] {e.Data}");
                    if (!string.IsNullOrWhiteSpace(e.Data))
                        _lastStderr = e.Data;
                }
            };
            _process.Exited += (_, _) =>
            {
                var exitCode = 0;
                try { exitCode = _process?.ExitCode ?? 0; } catch { }

                StopHealthCheck();
                _process = null;

                if (exitCode != 0)
                {
                    Status = ServerStatus.Error;
                    var stderr = _lastStderr.Trim();
                    ErrorMessage = stderr.Length > 0
                        ? (stderr.Length > 200 ? stderr.Substring(0, 200) : stderr)
                        : $"Exited ({exitCode})";
                }
                else
                {
                    Status = ServerStatus.Stopped;
                }
            };

            _process.Start();

            if (_jobHandle != IntPtr.Zero)
                AssignProcessToJobObject(_jobHandle, _process.Handle);

            _process.BeginOutputReadLine();
            _process.BeginErrorReadLine();

            StartHealthCheck();
        }
        catch (Exception ex)
        {
            Status = ServerStatus.Error;
            ErrorMessage = ex.Message;
        }
    }

    public void Stop()
    {
        StopHealthCheck();
        DisconnectNotificationWS();

        if (_process is { HasExited: false })
        {
            try
            {
                // On Windows, Process.Kill with entireProcessTree is the reliable way to stop.
                // There is no SIGTERM equivalent; CloseMainWindow doesn't work for console apps.
                _process.Kill();
                _process.WaitForExit(5000);
            }
            catch { }
        }

        _process = null;
        Status = ServerStatus.Stopped;
    }

    public async Task RestartAsync()
    {
        var oldProcess = _process;
        Stop();
        Status = ServerStatus.Starting;

        // Wait for the old process to fully exit so the port is freed.
        if (oldProcess is { HasExited: false })
        {
            await Task.Run(() =>
            {
                try { oldProcess.WaitForExit(10000); } catch { }
            });
        }

        Start();
    }

    private void StartHealthCheck()
    {
        _healthCheckTimer?.Dispose();
        _healthCheckTimer = new Timer(async _ => await CheckHealthAsync(), null,
            TimeSpan.FromMilliseconds(500), TimeSpan.FromSeconds(2));
    }

    private void StopHealthCheck()
    {
        _healthCheckTimer?.Dispose();
        _healthCheckTimer = null;
    }

    private async Task CheckHealthAsync()
    {
        if (_process == null || _process.HasExited)
            return;

        try
        {
            var response = await _httpClient.GetAsync($"http://localhost:{Port}/api/v1/health");
            if (response.IsSuccessStatusCode)
            {
                if (Status != ServerStatus.Running)
                    ConnectNotificationWS();
                Status = ServerStatus.Running;
            }
        }
        catch
        {
            // Server might still be booting; only update if not already running.
            if (Status == ServerStatus.Running)
                Status = ServerStatus.Starting;
        }
    }

    private void ConnectNotificationWS()
    {
        _notifyWsCts?.Cancel();
        _notifyWsCts = new CancellationTokenSource();
        var ct = _notifyWsCts.Token;
        _notifyReconnectDelayMs = 1000;
        _ = RunNotificationWSLoop(ct);
    }

    private void DisconnectNotificationWS()
    {
        _notifyWsCts?.Cancel();
        _notifyWsCts = null;
    }

    private async Task RunNotificationWSLoop(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var ws = new ClientWebSocket();
                _activeWs = ws;
                var uri = new Uri($"ws://localhost:{Port}/api/v1/notifications/ws");
                await ws.ConnectAsync(uri, ct);
                _notifyReconnectDelayMs = 1000;
                // WS connected — stop polling and mark server as running.
                StopHealthCheck();
                Status = ServerStatus.Running;

                // Send post-update "installed" notification if the app was just updated.
                SendPostUpdateNotification();

                var buffer = new byte[4096];
                while (ws.State == WebSocketState.Open && !ct.IsCancellationRequested)
                {
                    var result = await ws.ReceiveAsync(new ArraySegment<byte>(buffer), ct);
                    if (result.MessageType == WebSocketMessageType.Close)
                        break;
                    if (result.MessageType == WebSocketMessageType.Text)
                    {
                        var text = Encoding.UTF8.GetString(buffer, 0, result.Count);
                        HandleNotificationMessage(text);
                    }
                }
            }
            catch (OperationCanceledException)
            {
                break;
            }
            catch
            {
                // Connection failed or dropped — update status and reconnect with backoff
                Status = _process is { HasExited: false } ? ServerStatus.Starting : ServerStatus.Stopped;
            }

            if (ct.IsCancellationRequested) break;
            try { await Task.Delay(_notifyReconnectDelayMs, ct); }
            catch (OperationCanceledException) { break; }
            _notifyReconnectDelayMs = Math.Min(_notifyReconnectDelayMs * 2, 30_000);
        }
    }

    private void HandleNotificationMessage(string text)
    {
        try
        {
            using var doc = JsonDocument.Parse(text);
            var root = doc.RootElement;
            if (root.GetProperty("type").GetString() != "notification") return;
            var data = root.GetProperty("data");
            var title = data.GetProperty("title").GetString() ?? "Buildermark";
            var body = data.GetProperty("body").GetString() ?? "";
            string? url = null;
            if (data.TryGetProperty("url", out var urlProp))
                url = urlProp.GetString();
            NotificationReceived?.Invoke(title, body, url);
        }
        catch { }
    }

    /// <summary>Sends a JSON message upstream through the notifications WebSocket.</summary>
    public async void SendWSMessage(string json)
    {
        var ws = _activeWs;
        if (ws == null || ws.State != WebSocketState.Open) return;
        try
        {
            var bytes = Encoding.UTF8.GetBytes(json);
            await ws.SendAsync(new ArraySegment<byte>(bytes), WebSocketMessageType.Text, true, CancellationToken.None);
        }
        catch { }
    }

    /// <summary>Notifies the server of an update status change.</summary>
    public void SendUpdateStatus(string state, string version, string? previousVersion = null)
    {
        var prev = previousVersion != null ? $",\"previousVersion\":\"{previousVersion}\"" : "";
        var message = $"{{\"type\":\"update_status\",\"data\":{{\"state\":\"{state}\",\"version\":\"{version}\",\"platform\":\"windows\"{prev}}}}}";
        SendWSMessage(message);
    }

    private void SendPostUpdateNotification()
    {
        var previousVersion = PreferencesManager.GetString("previousVersion", "");
        if (string.IsNullOrEmpty(previousVersion)) return;

        PreferencesManager.SetString("previousVersion", "");
        var currentVersion = System.Reflection.Assembly.GetExecutingAssembly().GetName().Version?.ToString(3) ?? "";
        if (!string.IsNullOrEmpty(currentVersion))
            SendUpdateStatus("installed", currentVersion, previousVersion);
    }

    private static string? ResolveServerBinary()
    {
        var exeDir = AppContext.BaseDirectory;

        // 1. App resources directory (same folder as the exe).
        var alongside = Path.Combine(exeDir, "buildermark-server.exe");
        if (File.Exists(alongside))
            return alongside;

        // 2. System PATH.
        var pathDirs = Environment.GetEnvironmentVariable("PATH")?.Split(Path.PathSeparator) ?? Array.Empty<string>();
        foreach (var dir in pathDirs)
        {
            var candidate = Path.Combine(dir, "buildermark-server.exe");
            if (File.Exists(candidate))
                return candidate;
        }

        return null;
    }

    private static string ResolveDBPath()
    {
        var envPath = Environment.GetEnvironmentVariable("BUILDERMARK_LOCAL_DB_PATH");
        if (!string.IsNullOrEmpty(envPath))
            return envPath;

        var appData = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
        return Path.Combine(appData, "Buildermark", "local.db");
    }

    private void OnPropertyChanged([CallerMemberName] string? name = null)
    {
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
    }

    public void Dispose()
    {
        Stop();
        _httpClient.Dispose();
        if (_jobHandle != IntPtr.Zero)
            CloseHandle(_jobHandle);
    }

    // -- Win32 Job Object interop --

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode)]
    private static extern nint CreateJobObject(nint lpJobAttributes, string? lpName);

    [DllImport("kernel32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetInformationJobObject(nint hJob, int jobObjectInfoClass, nint lpJobObjectInfo, uint cbJobObjectInfoLength);

    [DllImport("kernel32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AssignProcessToJobObject(nint hJob, nint hProcess);

    [DllImport("kernel32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseHandle(nint hObject);

    [StructLayout(LayoutKind.Sequential)]
    private struct JOBOBJECT_BASIC_LIMIT_INFORMATION
    {
        public long PerProcessUserTimeLimit;
        public long PerJobUserTimeLimit;
        public uint LimitFlags;
        public nuint MinimumWorkingSetSize;
        public nuint MaximumWorkingSetSize;
        public uint ActiveProcessLimit;
        public nint Affinity;
        public uint PriorityClass;
        public uint SchedulingClass;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct IO_COUNTERS
    {
        public ulong ReadOperationCount;
        public ulong WriteOperationCount;
        public ulong OtherOperationCount;
        public ulong ReadTransferCount;
        public ulong WriteTransferCount;
        public ulong OtherTransferCount;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct JOBOBJECT_EXTENDED_LIMIT_INFORMATION
    {
        public JOBOBJECT_BASIC_LIMIT_INFORMATION BasicLimitInformation;
        public IO_COUNTERS IoInfo;
        public nuint ProcessMemoryLimit;
        public nuint JobMemoryLimit;
        public nuint PeakProcessMemoryUsed;
        public nuint PeakJobMemoryUsed;
    }
}
