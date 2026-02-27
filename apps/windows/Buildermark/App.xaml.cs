using System;
using System.Diagnostics;
using System.Windows;
using System.Windows.Input;
using Hardcodet.Wpf.TaskbarNotification;

namespace Buildermark;

public partial class App : Application
{
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

    public void QuitApplication()
    {
        _serverManager?.Stop();
        _trayIcon?.Dispose();
        _updaterManager?.Dispose();
        Shutdown();
    }

    protected override void OnExit(ExitEventArgs e)
    {
        _serverManager?.Stop();
        _trayIcon?.Dispose();
        _updaterManager?.Dispose();
        base.OnExit(e);
    }

    public static void OpenInBrowser(string url)
    {
        Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
    }
}
