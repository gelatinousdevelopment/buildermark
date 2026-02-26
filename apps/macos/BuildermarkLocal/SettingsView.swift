import SwiftUI

struct SettingsView: View {
    var body: some View {
        Form {
            Section("Links") {
                Link("buildermark.dev", destination: URL(string: "https://buildermark.dev")!)
            }
        }
        .formStyle(.grouped)
        .frame(width: 350, height: 150)
    }
}
