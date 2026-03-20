using System;
using System.ComponentModel;
using System.Drawing;
using System.Reflection;
using System.Windows.Forms;

namespace Buildermark;

sealed class SettingsForm : Form
{
    private readonly ServerManager _serverManager;
    private readonly UpdaterManager _updaterManager;

    private readonly TabControl _tabControl;
    private readonly Panel _statusDot;
    private readonly Label _statusLabel;
    private readonly CheckBox _autoInstallCheckBox;
    private Color _statusColor = Color.Gray;

    public SettingsForm(ServerManager serverManager, UpdaterManager updaterManager)
    {
        _serverManager = serverManager;
        _updaterManager = updaterManager;

        Text = "Buildermark Settings";
        Width = 400;
        Height = 380;
        FormBorderStyle = FormBorderStyle.FixedDialog;
        MaximizeBox = false;
        MinimizeBox = false;
        StartPosition = FormStartPosition.CenterScreen;

        // Load icon from embedded resource
        try
        {
            var stream = Assembly.GetExecutingAssembly()
                .GetManifestResourceStream("Buildermark.Resources.buildermark.ico");
            if (stream != null)
                Icon = new Icon(stream);
        }
        catch { }

        _tabControl = new TabControl { Dock = DockStyle.Fill };

        // ---- General Tab ----
        var generalTab = new TabPage("General") { Padding = new Padding(16) };

        var generalPanel = new FlowLayoutPanel
        {
            Dock = DockStyle.Fill,
            FlowDirection = FlowDirection.TopDown,
            WrapContents = false,
            AutoScroll = true,
        };

        // Server link
        var linkPanel = new FlowLayoutPanel
        {
            FlowDirection = FlowDirection.LeftToRight,
            AutoSize = true,
            WrapContents = false,
            Margin = new Padding(0, 0, 0, 4),
        };
        linkPanel.Controls.Add(new Label
        {
            Text = "Buildermark:",
            AutoSize = true,
            Margin = new Padding(0, 4, 4, 0),
        });
        var serverLink = new LinkLabel
        {
            Text = $"http://localhost:{ServerManager.Port}",
            AutoSize = true,
            Margin = new Padding(0, 4, 0, 0),
        };
        serverLink.LinkClicked += (_, _) => TrayApplicationContext.OpenInBrowser($"http://localhost:{ServerManager.Port}");
        linkPanel.Controls.Add(serverLink);
        generalPanel.Controls.Add(linkPanel);

        // Status row
        var statusPanel = new FlowLayoutPanel
        {
            FlowDirection = FlowDirection.LeftToRight,
            AutoSize = true,
            WrapContents = false,
            Margin = new Padding(0, 0, 0, 4),
        };
        _statusDot = new Panel
        {
            Size = new Size(12, 12),
            Margin = new Padding(0, 4, 4, 0),
            BackColor = Color.Transparent,
        };
        _statusDot.Paint += (_, e) =>
        {
            using var brush = new SolidBrush(_statusColor);
            e.Graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.AntiAlias;
            e.Graphics.FillEllipse(brush, 1, 1, 10, 10);
        };
        statusPanel.Controls.Add(_statusDot);
        _statusLabel = new Label { AutoSize = true, Margin = new Padding(0, 2, 0, 0) };
        statusPanel.Controls.Add(_statusLabel);
        generalPanel.Controls.Add(statusPanel);

        // Restart button
        var restartButton = new Button
        {
            Text = "Restart Server",
            AutoSize = true,
            Margin = new Padding(0, 4, 0, 12),
        };
        restartButton.Click += async (_, _) => await _serverManager.RestartAsync();
        generalPanel.Controls.Add(restartButton);

        // Separator
        generalPanel.Controls.Add(new Label
        {
            BorderStyle = BorderStyle.Fixed3D,
            AutoSize = false,
            Height = 2,
            Width = 320,
            Margin = new Padding(0, 0, 0, 12),
        });

        // Start at login
        var startAtLoginCb = new CheckBox
        {
            Text = "Start at login",
            AutoSize = true,
            Checked = PreferencesManager.GetBool("startAtLogin", true),
            Margin = new Padding(0, 0, 0, 4),
        };
        startAtLoginCb.CheckedChanged += (_, _) =>
        {
            PreferencesManager.SetBool("startAtLogin", startAtLoginCb.Checked);
            PreferencesManager.StartAtLogin = startAtLoginCb.Checked;
        };
        generalPanel.Controls.Add(startAtLoginCb);

        // Notifications
        var notificationsCb = new CheckBox
        {
            Text = "Enable notifications",
            AutoSize = true,
            Checked = PreferencesManager.GetBool("notificationsEnabled", true),
            Margin = new Padding(0, 0, 0, 0),
        };
        notificationsCb.CheckedChanged += (_, _) =>
        {
            PreferencesManager.SetBool("notificationsEnabled", notificationsCb.Checked);
        };
        generalPanel.Controls.Add(notificationsCb);
        generalPanel.Controls.Add(new Label
        {
            Text = "Show notifications for new commits and completed tasks.",
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Font = new Font(Font.FontFamily, 8f),
            Margin = new Padding(20, 0, 0, 4),
        });

        // Hide icon
        var hideIconCb = new CheckBox
        {
            Text = "Hide system tray icon",
            AutoSize = true,
            Checked = PreferencesManager.GetBool("hideMenuBarIcon", false),
            Margin = new Padding(0, 0, 0, 0),
        };
        hideIconCb.CheckedChanged += (_, _) =>
        {
            PreferencesManager.SetBool("hideMenuBarIcon", hideIconCb.Checked);
        };
        generalPanel.Controls.Add(hideIconCb);
        generalPanel.Controls.Add(new Label
        {
            Text = "Relaunch the app for this to take effect.",
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Font = new Font(Font.FontFamily, 8f),
            Margin = new Padding(20, 0, 0, 0),
        });
        generalPanel.Controls.Add(new Label
        {
            Text = "When hidden, launch app to show settings.",
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Font = new Font(Font.FontFamily, 8f),
            Margin = new Padding(20, 0, 0, 0),
        });

        generalTab.Controls.Add(generalPanel);

        // Sync start-at-login on first appearance
        PreferencesManager.SyncDefaults();

        // ---- Updates Tab ----
        var updatesTab = new TabPage("Updates") { Padding = new Padding(16) };
        var updatesPanel = new FlowLayoutPanel
        {
            Dock = DockStyle.Fill,
            FlowDirection = FlowDirection.TopDown,
            WrapContents = false,
        };

        var autoCheckCb = new CheckBox
        {
            Text = "Automatically check for updates",
            AutoSize = true,
            Checked = _updaterManager.AutomaticallyChecksForUpdates,
            Margin = new Padding(0, 0, 0, 4),
        };

        _autoInstallCheckBox = new CheckBox
        {
            Text = "Automatically install updates",
            AutoSize = true,
            Checked = _updaterManager.AutomaticallyInstallsUpdates,
            Enabled = autoCheckCb.Checked,
            Margin = new Padding(0, 0, 0, 12),
        };

        autoCheckCb.CheckedChanged += (_, _) =>
        {
            _updaterManager.AutomaticallyChecksForUpdates = autoCheckCb.Checked;
            _autoInstallCheckBox.Enabled = autoCheckCb.Checked;
        };
        _autoInstallCheckBox.CheckedChanged += (_, _) =>
        {
            _updaterManager.AutomaticallyInstallsUpdates = _autoInstallCheckBox.Checked;
        };

        updatesPanel.Controls.Add(autoCheckCb);
        updatesPanel.Controls.Add(_autoInstallCheckBox);

        var checkUpdatesButton = new Button
        {
            Text = "Check for Updates...",
            AutoSize = true,
        };
        checkUpdatesButton.Click += (_, _) => _updaterManager.CheckForUpdates();
        updatesPanel.Controls.Add(checkUpdatesButton);

        updatesTab.Controls.Add(updatesPanel);

        // ---- About Tab ----
        var aboutTab = new TabPage("About") { Padding = new Padding(16) };
        var aboutPanel = new Panel { Dock = DockStyle.Fill };

        // App icon
        var iconBox = new PictureBox
        {
            Size = new Size(64, 64),
            SizeMode = PictureBoxSizeMode.StretchImage,
            Anchor = AnchorStyles.Top,
        };
        try
        {
            var iconStream = Assembly.GetExecutingAssembly()
                .GetManifestResourceStream("Buildermark.Resources.buildermark.ico");
            if (iconStream != null)
                iconBox.Image = new Icon(iconStream, 64, 64).ToBitmap();
        }
        catch { }
        aboutPanel.Controls.Add(iconBox);

        // App name
        var nameLabel = new Label
        {
            Text = "Buildermark",
            AutoSize = true,
            Font = new Font(Font.FontFamily, 14f, FontStyle.Bold),
            Anchor = AnchorStyles.Top,
        };
        aboutPanel.Controls.Add(nameLabel);

        // Version
        var version = Assembly.GetExecutingAssembly().GetName().Version;
        var fileVersion = Assembly.GetExecutingAssembly()
            .GetCustomAttribute<AssemblyFileVersionAttribute>()?.Version ?? "1.0.0.0";
        var versionLabel = new Label
        {
            Text = $"Version {version?.ToString(3) ?? "1.0.0"} ({fileVersion})",
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Anchor = AnchorStyles.Top,
        };
        aboutPanel.Controls.Add(versionLabel);

        // Copyright
        var copyright = Assembly.GetExecutingAssembly()
            .GetCustomAttribute<AssemblyCopyrightAttribute>()?.Copyright
            ?? "\u00A9 2026 Gelatinous Development Studio";
        var copyrightLabel = new Label
        {
            Text = copyright,
            AutoSize = true,
            ForeColor = SystemColors.GrayText,
            Font = new Font(Font.FontFamily, 8f),
            Anchor = AnchorStyles.Top,
        };
        aboutPanel.Controls.Add(copyrightLabel);

        // Website link
        var websiteLink = new LinkLabel
        {
            Text = "https://buildermark.dev",
            AutoSize = true,
            Anchor = AnchorStyles.Top,
        };
        websiteLink.LinkClicked += (_, _) => TrayApplicationContext.OpenInBrowser("https://buildermark.dev");
        aboutPanel.Controls.Add(websiteLink);

        // Center all about controls horizontally, stack vertically
        aboutPanel.Resize += (_, _) => CenterAboutControls(aboutPanel, iconBox, nameLabel, versionLabel, copyrightLabel, websiteLink);
        aboutPanel.Layout += (_, _) => CenterAboutControls(aboutPanel, iconBox, nameLabel, versionLabel, copyrightLabel, websiteLink);

        aboutTab.Controls.Add(aboutPanel);

        // Add tabs
        _tabControl.TabPages.Add(generalTab);
        _tabControl.TabPages.Add(updatesTab);
        _tabControl.TabPages.Add(aboutTab);
        Controls.Add(_tabControl);

        // Subscribe to server status changes
        _serverManager.PropertyChanged += ServerManager_PropertyChanged;
        UpdateServerStatus();
    }

    private void ServerManager_PropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (InvokeRequired)
            Invoke(UpdateServerStatus);
        else
            UpdateServerStatus();
    }

    private void UpdateServerStatus()
    {
        _statusLabel.Text = _serverManager.StatusText;
        _statusColor = _serverManager.Status switch
        {
            ServerStatus.Stopped => Color.Gray,
            ServerStatus.Starting => Color.Orange,
            ServerStatus.Running => Color.Green,
            ServerStatus.Error => Color.Red,
            _ => Color.Gray
        };
        _statusDot.Invalidate();
    }

    public void SelectTab(string tabHeader)
    {
        foreach (TabPage tab in _tabControl.TabPages)
        {
            if (tab.Text == tabHeader)
            {
                _tabControl.SelectedTab = tab;
                break;
            }
        }
    }

    private static void CenterAboutControls(Panel panel, params Control[] controls)
    {
        int y = 20;
        int spacing = 4;
        foreach (var ctrl in controls)
        {
            ctrl.Left = (panel.ClientSize.Width - ctrl.Width) / 2;
            ctrl.Top = y;
            y += ctrl.Height + spacing;
        }
    }

    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        _serverManager.PropertyChanged -= ServerManager_PropertyChanged;
        base.OnFormClosing(e);
    }
}
