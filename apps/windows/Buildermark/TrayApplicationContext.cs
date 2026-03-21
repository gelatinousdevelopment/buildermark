using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.IO.Pipes;
using System.Reflection;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
using Microsoft.Win32;

namespace Buildermark;

sealed class TrayApplicationContext : ApplicationContext
{
    private const string PipeName = "Buildermark_SingleInstance";

    private readonly NotifyIcon _trayIcon;
    private readonly ServerManager _serverManager;
    private readonly UpdaterManager _updaterManager;
    private readonly CancellationTokenSource _pipeCts;
    private readonly SynchronizationContext _syncContext;
    private SettingsForm? _settingsForm;

    public TrayApplicationContext(string[] args)
    {
        _syncContext = SynchronizationContext.Current!;
        _pipeCts = new CancellationTokenSource();

        _serverManager = new ServerManager();
        _updaterManager = new UpdaterManager();
        _updaterManager.SetServerManager(_serverManager);

        // Build tray icon
        _trayIcon = new NotifyIcon
        {
            Icon = LoadEmbeddedIcon(),
            Text = "Buildermark",
            Visible = !PreferencesManager.GetBool("hideMenuBarIcon", false),
        };

        var menu = new ContextMenuStrip();
        var statusItem = new ToolStripMenuItem("Server Status") { Enabled = false };
        menu.Items.Add(statusItem);
        menu.Items.Add(new ToolStripSeparator());
        var openItem = new ToolStripMenuItem("Open Buildermark");
        openItem.Click += (_, _) => OpenInBrowser($"http://localhost:{ServerManager.Port}");
        menu.Items.Add(openItem);
        menu.Items.Add(new ToolStripSeparator());
        var settingsItem = new ToolStripMenuItem("Settings...");
        settingsItem.Click += (_, _) => ShowSettingsWindow();
        menu.Items.Add(settingsItem);
        var quitItem = new ToolStripMenuItem("Quit Buildermark");
        quitItem.Click += (_, _) => QuitApplication();
        menu.Items.Add(quitItem);

        menu.Opening += (_, _) =>
        {
            statusItem.Text = _serverManager.StatusText;
        };

        _trayIcon.ContextMenuStrip = menu;
        _trayIcon.Click += (_, e) =>
        {
            if (e is MouseEventArgs me && me.Button == MouseButtons.Left)
                OpenInBrowser($"http://localhost:{ServerManager.Port}");
        };

        // Wire notifications
        _serverManager.NotificationReceived += OnServerNotification;
        _serverManager.Start();

        // Named pipe listener for second instances
        ListenForSecondInstance(_pipeCts.Token);

        RegisterUrlScheme();
        DetectPostUpdate();

        // Handle URL arg if launched with one
        var launchUrl = Program.FindUrlArg(args);
        if (launchUrl != null && launchUrl.Contains("settings/update"))
            ShowSettingsWindow("Updates");
        else
            ShowSettingsWindow();
    }

    private static Icon LoadEmbeddedIcon()
    {
        var stream = Assembly.GetExecutingAssembly()
            .GetManifestResourceStream("Buildermark.Resources.tray-icon.png");
        if (stream != null)
        {
            using var bitmap = new Bitmap(stream);
            return Icon.FromHandle(bitmap.GetHicon());
        }

        var icoStream = Assembly.GetExecutingAssembly()
            .GetManifestResourceStream("Buildermark.Resources.buildermark.ico");
        return icoStream != null ? new Icon(icoStream) : SystemIcons.Application;
    }

    private void OnServerNotification(string title, string body, string? url)
    {
        if (!PreferencesManager.GetBool("notificationsEnabled", true))
            return;

        _syncContext.Post(_ =>
        {
            _trayIcon.ShowBalloonTip(3000, title, body, ToolTipIcon.Info);
        }, null);
    }

    public void ShowSettingsWindow(string? tabName = null)
    {
        if (_settingsForm is { IsDisposed: false })
        {
            if (tabName != null)
                _settingsForm.SelectTab(tabName);
            _settingsForm.Activate();
            return;
        }

        _settingsForm = new SettingsForm(_serverManager, _updaterManager);
        _settingsForm.Show();
        if (tabName != null)
            _settingsForm.SelectTab(tabName);
        _settingsForm.Activate();
    }

    private void QuitApplication()
    {
        _serverManager.Stop();
        _trayIcon.Visible = false;
        _trayIcon.Dispose();
        _updaterManager.Dispose();
        _pipeCts.Cancel();
        _pipeCts.Dispose();
        Application.ExitThread();
    }

    private async void ListenForSecondInstance(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                using var server = new NamedPipeServerStream(PipeName, PipeDirection.In, 1,
                    PipeTransmissionMode.Byte, System.IO.Pipes.PipeOptions.Asynchronous);
                await server.WaitForConnectionAsync();
                using var reader = new StreamReader(server);
                var message = await reader.ReadToEndAsync();
                _syncContext.Post(_ =>
                {
                    if (message.StartsWith("show:") && message.Contains("settings/update"))
                        ShowSettingsWindow("Updates");
                    else
                        ShowSettingsWindow();
                }, null);
            }
            catch (OperationCanceledException) { break; }
            catch { }
        }
    }

    private static void RegisterUrlScheme()
    {
        if (PreferencesManager.GetBool("urlSchemeRegistered", false))
            return;

        try
        {
            var exePath = Process.GetCurrentProcess().MainModule?.FileName;
            if (exePath == null) return;

            using var key = Registry.CurrentUser.CreateSubKey(@"Software\Classes\buildermark");
            key.SetValue("", "URL:Buildermark Protocol");
            key.SetValue("URL Protocol", "");

            using var shellKey = key.CreateSubKey(@"shell\open\command");
            shellKey.SetValue("", $"\"{exePath}\" \"%1\"");

            PreferencesManager.SetBool("urlSchemeRegistered", true);
        }
        catch { }
    }

    private static void DetectPostUpdate()
    {
        try
        {
            var currentVersion = Assembly.GetExecutingAssembly().GetName().Version?.ToString(3) ?? "";
            var lastKnownVersion = PreferencesManager.GetString("lastKnownVersion", "");

            if (!string.IsNullOrEmpty(lastKnownVersion) && lastKnownVersion != currentVersion)
            {
                PreferencesManager.SetString("previousVersion", lastKnownVersion);
            }
            PreferencesManager.SetString("lastKnownVersion", currentVersion);
        }
        catch { }
    }

    internal static void OpenInBrowser(string url)
    {
        Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
    }
}
