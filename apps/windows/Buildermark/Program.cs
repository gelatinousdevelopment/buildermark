using System;
using System.IO;
using System.IO.Pipes;
using System.Threading;
using System.Windows.Forms;

namespace Buildermark;

static class Program
{
    private const string PipeName = "Buildermark_SingleInstance";

    [STAThread]
    static void Main(string[] args)
    {
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);

        using var mutex = new Mutex(true, PipeName, out bool createdNew);
        if (!createdNew)
        {
            var urlArg = FindUrlArg(args);
            SignalExistingInstance(urlArg != null ? $"show:{urlArg}" : "show");
            return;
        }

        Application.Run(new TrayApplicationContext(args));
    }

    internal static string? FindUrlArg(string[] args)
    {
        foreach (var arg in args)
        {
            if (arg.StartsWith("buildermark://", StringComparison.OrdinalIgnoreCase))
                return arg;
        }
        return null;
    }

    internal static void SignalExistingInstance(string message = "show")
    {
        try
        {
            using var client = new NamedPipeClientStream(".", PipeName, PipeDirection.Out);
            client.Connect(timeout: 1000);
            using var writer = new StreamWriter(client);
            writer.Write(message);
        }
        catch { }
    }
}
