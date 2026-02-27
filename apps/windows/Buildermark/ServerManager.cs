using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
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
    public const int Port = 7022;

    private Process? _process;
    private Timer? _healthCheckTimer;
    private readonly HttpClient _httpClient = new() { Timeout = TimeSpan.FromSeconds(5) };
    private readonly nint _jobHandle;
    private ServerStatus _status = ServerStatus.Stopped;
    private string _errorMessage = "";
    private string _lastStderr = "";

    public ServerManager()
    {
        _jobHandle = CreateJobObject(nint.Zero, null);
        if (_jobHandle != nint.Zero)
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
            Arguments = $"-addr :{Port} -db \"{dbPath}\"",
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
                        ? (stderr.Length > 200 ? stderr[..200] : stderr)
                        : $"Exited ({exitCode})";
                }
                else
                {
                    Status = ServerStatus.Stopped;
                }
            };

            _process.Start();

            if (_jobHandle != nint.Zero)
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

        if (_process is { HasExited: false })
        {
            try
            {
                // On Windows, Process.Kill with entireProcessTree is the reliable way to stop.
                // There is no SIGTERM equivalent; CloseMainWindow doesn't work for console apps.
                _process.Kill(entireProcessTree: true);
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
            var response = await _httpClient.GetAsync($"http://localhost:{Port}/api/v1/settings");
            if (response.IsSuccessStatusCode)
            {
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

    private static string? ResolveServerBinary()
    {
        var exeDir = AppContext.BaseDirectory;

        // 1. App resources directory (same folder as the exe).
        var alongside = Path.Combine(exeDir, "buildermark-server.exe");
        if (File.Exists(alongside))
            return alongside;

        // 2. System PATH.
        var pathDirs = Environment.GetEnvironmentVariable("PATH")?.Split(Path.PathSeparator) ?? [];
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
        if (_jobHandle != nint.Zero)
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
