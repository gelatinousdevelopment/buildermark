using Microsoft.Win32;

namespace Buildermark;

/// <summary>
/// Persists preferences in the Windows registry under HKCU\Software\Buildermark.
/// </summary>
public static class PreferencesManager
{
    private const string RegistryKeyPath = @"Software\Buildermark";
    private const string StartupRegistryKeyPath = @"Software\Microsoft\Windows\CurrentVersion\Run";
    private const string AppName = "Buildermark";

    public static bool GetBool(string key, bool defaultValue)
    {
        using var regKey = Registry.CurrentUser.OpenSubKey(RegistryKeyPath);
        var value = regKey?.GetValue(key);
        if (value is int intVal)
            return intVal != 0;
        return defaultValue;
    }

    public static void SetBool(string key, bool value)
    {
        using var regKey = Registry.CurrentUser.CreateSubKey(RegistryKeyPath);
        regKey.SetValue(key, value ? 1 : 0, RegistryValueKind.DWord);
    }

    public static bool StartAtLogin
    {
        get
        {
            using var key = Registry.CurrentUser.OpenSubKey(StartupRegistryKeyPath);
            return key?.GetValue(AppName) != null;
        }
        set
        {
            using var key = Registry.CurrentUser.OpenSubKey(StartupRegistryKeyPath, writable: true);
            if (key == null) return;

            if (value)
            {
                var exePath = System.Diagnostics.Process.GetCurrentProcess().MainModule?.FileName;
                if (exePath != null)
                    key.SetValue(AppName, $"\"{exePath}\"");
            }
            else
            {
                key.DeleteValue(AppName, throwOnMissingValue: false);
            }
        }
    }

    /// <summary>
    /// Ensures preferences that have OS side-effects are synced on first launch.
    /// </summary>
    public static void SyncDefaults()
    {
        // If startAtLogin has never been set, default to true and register.
        using var regKey = Registry.CurrentUser.OpenSubKey(RegistryKeyPath);
        if (regKey?.GetValue("startAtLogin") == null)
        {
            SetBool("startAtLogin", true);
            StartAtLogin = true;
        }
        else
        {
            // Sync the OS state to match the persisted preference.
            StartAtLogin = GetBool("startAtLogin", true);
        }
    }
}
