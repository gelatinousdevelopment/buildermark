using System;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using System.Windows.Forms;
using NetSparkleUpdater;
using NetSparkleUpdater.SignatureVerifiers;

namespace Buildermark;

public sealed class UpdaterManager : INotifyPropertyChanged, IDisposable
{
    private const string AppcastUrl = "https://buildermark.dev/appcast.xml";
    private const string EdDSAPublicKey = "ej6jDgZczuarlscgV2RMH0JZoFHZzokVys3/YfRelAY=";

    private readonly SparkleUpdater _updater;
    private bool _canCheckForUpdates = true;
    private bool _automaticallyInstallsUpdates;
    private ServerManager? _serverManager;

    public event PropertyChangedEventHandler? PropertyChanged;

    /// <summary>Raised when Sparkle finds an available update.</summary>
    public event Action<string>? UpdateAvailable;

    public UpdaterManager()
    {
        _updater = new SparkleUpdater(AppcastUrl, new Ed25519Checker(NetSparkleUpdater.Enums.SecurityMode.Strict, EdDSAPublicKey))
        {
            UIFactory = new NetSparkleUpdater.UI.WinForms.UIFactory(),
            RelaunchAfterUpdate = true,
        };

        _updater.UpdateDetected += (_, e) =>
        {
            var version = e.LatestVersion?.Version;
            if (version != null)
            {
                UpdateAvailable?.Invoke(version);
                _serverManager?.SendUpdateStatus("available", version);
            }
        };

        _updater.StartLoop(true, true);
    }

    /// <summary>Sets the server manager reference for sending WS update notifications.</summary>
    public void SetServerManager(ServerManager serverManager)
    {
        _serverManager = serverManager;
    }

    public bool AutomaticallyChecksForUpdates
    {
        get => _updater.IsUpdateLoopRunning;
        set
        {
            if (value)
            {
                if (!_updater.IsUpdateLoopRunning)
                    _updater.StartLoop(true, true);
            }
            else
            {
                if (_updater.IsUpdateLoopRunning)
                    _updater.StopLoop();
            }
            OnPropertyChanged();
        }
    }

    public bool AutomaticallyInstallsUpdates
    {
        get => _automaticallyInstallsUpdates;
        set
        {
            _automaticallyInstallsUpdates = value;
            OnPropertyChanged();
        }
    }

    public bool CanCheckForUpdates
    {
        get => _canCheckForUpdates;
        private set { _canCheckForUpdates = value; OnPropertyChanged(); }
    }

    public async void CheckForUpdates()
    {
        CanCheckForUpdates = false;
        try
        {
            // CheckForUpdatesAtUserRequest shows the "Checking for updates..." dialog and
            // then dispatches the appropriate UI through UIFactory for every outcome:
            // update available, version up to date, or appcast download error.
            await _updater.CheckForUpdatesAtUserRequest();
        }
        catch (Exception ex)
        {
            MessageBox.Show(
                $"An error occurred while checking for updates:\n\n{ex.Message}",
                "Buildermark",
                MessageBoxButtons.OK,
                MessageBoxIcon.Error);
        }
        finally
        {
            CanCheckForUpdates = true;
        }
    }

    private void OnPropertyChanged([CallerMemberName] string? name = null)
    {
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
    }

    public void Dispose()
    {
        _updater.Dispose();
    }
}
