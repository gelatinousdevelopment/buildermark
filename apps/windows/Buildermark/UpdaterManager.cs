using System;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using NetSparkleUpdater;
using NetSparkleUpdater.SignatureVerifiers;

namespace Buildermark;

public sealed class UpdaterManager : INotifyPropertyChanged, IDisposable
{
    private const string AppcastUrl = "https://buildermark.dev/appcast.xml";
    private const string EdDSAPublicKey = "ej6jDgZczuarlscgV2RMH0JZoFHZzokVys3/YfRelAY=";

    private readonly SparkleUpdater _updater;
    private bool _canCheckForUpdates = true;

    public event PropertyChangedEventHandler? PropertyChanged;

    public UpdaterManager()
    {
        _updater = new SparkleUpdater(AppcastUrl, new Ed25519Checker(NetSparkleUpdater.Enums.SecurityMode.Strict, EdDSAPublicKey))
        {
            UIFactory = new NetSparkleUpdater.UI.WPF.UIFactory(null),
            RelaunchAfterUpdate = true,
        };

        _updater.StartLoop(true, true);
    }

    public bool AutomaticallyChecksForUpdates
    {
        get => _updater.CheckServerFileName != null;
        set
        {
            // NetSparkle handles auto-check via the loop configuration.
            // Re-start the loop with the new setting.
            _updater.StartLoop(value, true);
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
            var updateInfo = await _updater.CheckForUpdatesQuietly();
            if (updateInfo?.Status == NetSparkleUpdater.Enums.UpdateStatus.UpdateAvailable)
            {
                _updater.ShowUpdateNeededUI(updateInfo.Updates);
            }
            else
            {
                _updater.ShowUpdateNeededUI(null);
            }
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
