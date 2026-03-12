using System;
using System.Diagnostics;
using System.IO;
using System.IO.Pipes;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Input;
using Hardcodet.Wpf.TaskbarNotification;

namespace Buildermark;

public partial class App : Application
{
    private const string PipeName = "Buildermark_SingleInstance";
    private static Mutex? _mutex;
    private CancellationTokenSource? _pipeCts;
    private TaskbarIcon? _trayIcon;
    private ServerManager? _serverManager;
    private UpdaterManager? _updaterManager;
    private SettingsWindow? _settingsWindow;

    public static RoutedCommand OpenBuildermarkCommand { get; } = new();

    public ServerManager ServerManager => _serverManager!;
    public UpdaterManager UpdaterManager => _updaterManager!;

    protected override void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);

        _mutex = new Mutex(true, PipeName, out bool createdNew);
        if (!createdNew)
        {
            SignalExistingInstance();
            Shutdown();
            return;
        }

        _pipeCts = new CancellationTokenSource();
        ListenForSecondInstance(_pipeCts.Token);

        _serverManager = new ServerManager();
        _updaterManager = new UpdaterManager();

        _trayIcon = (TaskbarIcon)FindResource("TrayIcon");

        // Wire up context menu events — XAML Click handlers need code-behind on App
        if (_trayIcon.ContextMenu is { } menu)
        {
            foreach (var item in menu.Items)
            {
                if (item is System.Windows.Controls.MenuItem mi)
                {
                    switch (mi.Header?.ToString())
                    {
                        case "Open Buildermark":
                            mi.Click += OpenBuildermark_Click;
                            break;
                        case "Settings...":
                            mi.Click += Settings_Click;
                            break;
                        case "Quit Buildermark":
                            mi.Click += Quit_Click;
                            break;
                    }
                }
            }

            // Update status item when menu opens
            menu.Opened += (_, _) => UpdateStatusMenuItem();
        }

        var hideIcon = PreferencesManager.GetBool("hideMenuBarIcon", false);
        if (hideIcon)
        {
            _trayIcon.Visibility = Visibility.Collapsed;
        }

        _serverManager.NotificationReceived += OnServerNotification;
        _serverManager.Start();
        ShowSettingsWindow();
    }

    private void UpdateStatusMenuItem()
    {
        if (_trayIcon?.ContextMenu?.Items[0] is System.Windows.Controls.MenuItem statusItem
            && _serverManager != null)
        {
            statusItem.Header = _serverManager.StatusText;
        }
    }

    private void OpenBuildermark_Click(object sender, RoutedEventArgs e)
    {
        OpenInBrowser($"http://localhost:{ServerManager.Port}");
    }

    private void Settings_Click(object sender, RoutedEventArgs e)
    {
        ShowSettingsWindow();
    }

    private void Quit_Click(object sender, RoutedEventArgs e)
    {
        QuitApplication();
    }

    public void ShowSettingsWindow()
    {
        if (_settingsWindow is { IsLoaded: true })
        {
            _settingsWindow.Activate();
            return;
        }

        _settingsWindow = new SettingsWindow();
        _settingsWindow.Show();
        _settingsWindow.Activate();
    }

    private void OnServerNotification(string title, string body, string? url)
    {
        if (!PreferencesManager.GetBool("notificationsEnabled", true))
            return;

        Dispatcher.Invoke(() =>
        {
            _trayIcon?.ShowBalloonTip(title, body, Hardcodet.Wpf.TaskbarNotification.BalloonIcon.Info);
        });
    }

    public void QuitApplication()
    {
        _serverManager?.Stop();
        _trayIcon?.Dispose();
        _updaterManager?.Dispose();
        Shutdown();
    }

    private static void SignalExistingInstance()
    {
        try
        {
            using var client = new NamedPipeClientStream(".", PipeName, PipeDirection.Out);
            client.Connect(timeout: 1000);
            using var writer = new StreamWriter(client);
            writer.Write("show");
        }
        catch { }
    }

    private async void ListenForSecondInstance(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var server = new NamedPipeServerStream(PipeName, PipeDirection.In, 1,
                    PipeTransmissionMode.Byte, System.IO.Pipes.PipeOptions.Asynchronous);
                await server.WaitForConnectionAsync(ct);
                Dispatcher.Invoke(ShowSettingsWindow);
            }
            catch (OperationCanceledException) { break; }
            catch { }
        }
    }

    protected override void OnExit(ExitEventArgs e)
    {
        _pipeCts?.Cancel();
        _pipeCts?.Dispose();
        _serverManager?.Stop();
        _trayIcon?.Dispose();
        _updaterManager?.Dispose();
        _mutex?.ReleaseMutex();
        _mutex?.Dispose();
        base.OnExit(e);
    }

    public static void OpenInBrowser(string url)
    {
        Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
    }
}
