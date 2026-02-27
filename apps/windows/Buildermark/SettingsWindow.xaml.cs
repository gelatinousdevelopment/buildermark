using System;
using System.ComponentModel;
using System.Reflection;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Media.Imaging;

namespace Buildermark;

public partial class SettingsWindow : Window
{
    private readonly ServerManager _serverManager;
    private readonly UpdaterManager _updaterManager;

    public SettingsWindow()
    {
        InitializeComponent();

        var app = (App)Application.Current;
        _serverManager = app.ServerManager;
        _updaterManager = app.UpdaterManager;

        // Server link
        ServerLinkText.Text = $"http://localhost:{ServerManager.Port}";

        // Load preferences
        StartAtLoginCheckBox.IsChecked = PreferencesManager.GetBool("startAtLogin", true);
        HideIconCheckBox.IsChecked = PreferencesManager.GetBool("hideMenuBarIcon", false);
        AutoUpdateCheckBox.IsChecked = _updaterManager.AutomaticallyChecksForUpdates;

        // Sync start-at-login on first appearance
        PreferencesManager.SyncDefaults();

        // Version info
        var version = Assembly.GetExecutingAssembly().GetName().Version;
        var fileVersion = Assembly.GetExecutingAssembly()
            .GetCustomAttribute<AssemblyFileVersionAttribute>()?.Version ?? "1.0.0.0";
        VersionText.Text = $"Version {version?.ToString(3) ?? "1.0.0"} ({fileVersion})";

        var copyright = Assembly.GetExecutingAssembly()
            .GetCustomAttribute<AssemblyCopyrightAttribute>()?.Copyright
            ?? "\u00A9 2026 Gelatinous Development Studio";
        CopyrightText.Text = copyright;

        // App icon for About tab
        try
        {
            var iconUri = new Uri("pack://application:,,,/Resources/buildermark.ico");
            AppIconImage.Source = new BitmapImage(iconUri);
        }
        catch { }

        // Subscribe to server status changes
        _serverManager.PropertyChanged += ServerManager_PropertyChanged;
        UpdateServerStatus();
    }

    private void ServerManager_PropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        Dispatcher.Invoke(UpdateServerStatus);
    }

    private void UpdateServerStatus()
    {
        StatusText.Text = _serverManager.StatusText;
        StatusIndicator.Fill = _serverManager.Status switch
        {
            ServerStatus.Stopped => Brushes.Gray,
            ServerStatus.Starting => Brushes.Orange,
            ServerStatus.Running => Brushes.Green,
            ServerStatus.Error => Brushes.Red,
            _ => Brushes.Gray
        };
    }

    private void ServerLink_Click(object sender, RoutedEventArgs e)
    {
        App.OpenInBrowser($"http://localhost:{ServerManager.Port}");
    }

    private async void RestartServer_Click(object sender, RoutedEventArgs e)
    {
        await _serverManager.RestartAsync();
    }

    private void StartAtLogin_Changed(object sender, RoutedEventArgs e)
    {
        var enabled = StartAtLoginCheckBox.IsChecked == true;
        PreferencesManager.SetBool("startAtLogin", enabled);
        PreferencesManager.StartAtLogin = enabled;
    }

    private void HideIcon_Changed(object sender, RoutedEventArgs e)
    {
        var hidden = HideIconCheckBox.IsChecked == true;
        PreferencesManager.SetBool("hideMenuBarIcon", hidden);
    }

    private void AutoUpdate_Changed(object sender, RoutedEventArgs e)
    {
        _updaterManager.AutomaticallyChecksForUpdates = AutoUpdateCheckBox.IsChecked == true;
    }

    private void CheckUpdates_Click(object sender, RoutedEventArgs e)
    {
        _updaterManager.CheckForUpdates();
    }

    private void WebsiteLink_Click(object sender, RoutedEventArgs e)
    {
        App.OpenInBrowser("https://buildermark.dev");
    }

    private void TabControl_SelectionChanged(object sender, SelectionChangedEventArgs e) { }

    protected override void OnClosing(CancelEventArgs e)
    {
        // Closing the settings window must NOT quit the app.
        _serverManager.PropertyChanged -= ServerManager_PropertyChanged;
        base.OnClosing(e);
    }
}
