using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Runtime.CompilerServices;
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
    private ServerStatus _status = ServerStatus.Stopped;
    private string _errorMessage = "";

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
                    Trace.WriteLine($"[server stderr] {e.Data}");
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
                    ErrorMessage = $"Exited ({exitCode})";
                }
                else
                {
                    Status = ServerStatus.Stopped;
                }
            };

            _process.Start();
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
    }
}
