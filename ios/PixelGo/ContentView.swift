import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        Group {
            if !model.didBootstrap {
                ProgressView("Opening PIXEL GO…")
            } else if model.isAuthenticated {
                HomeView()
            } else {
                AuthView()
            }
        }
    }
}

private struct AuthView: View {
    @EnvironmentObject private var model: AppModel
    @State private var email = ""
    @State private var password = ""
    @State private var createAccount = false

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Spacer()

            Text("PIXEL GO")
                .font(.system(size: 38, weight: .black, design: .rounded))
            Text("Your devices. One private transfer space.")
                .font(.title3)
                .foregroundStyle(.secondary)

            TextField("Email", text: $email)
                .textInputAutocapitalization(.never)
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .padding()
                .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 16))

            SecureField("Password", text: $password)
                .textContentType(createAccount ? .newPassword : .password)
                .padding()
                .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 16))

            Button {
                Task {
                    if createAccount {
                        await model.register(email: email, password: password)
                    } else {
                        await model.login(email: email, password: password)
                    }
                }
            } label: {
                Group {
                    if model.isLoading {
                        ProgressView()
                    } else {
                        Text(createAccount ? "CREATE ACCOUNT" : "SIGN IN")
                    }
                }
                .font(.headline)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 14)
            }
            .buttonStyle(.borderedProminent)
            .disabled(email.isEmpty || password.isEmpty || model.isLoading)

            Button(createAccount ? "Already have an account? Sign in" : "New to PIXEL GO? Create account") {
                createAccount.toggle()
                model.errorMessage = nil
            }
            .frame(maxWidth: .infinity)

            if let error = model.errorMessage {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            }

            Spacer()
        }
        .padding(28)
    }
}

private struct HomeView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationStack {
            List {
                brand
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)

                Section("YOUR DEVICES") {
                    if model.devices.isEmpty {
                        Text("No devices registered yet")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.devices) { device in
                            HStack(spacing: 12) {
                                Circle()
                                    .fill(model.onlineDeviceIDs.contains(device.id) ? Color.green : Color.gray)
                                    .frame(width: 8, height: 8)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(device.name)
                                        .font(.headline)
                                    Text(device.platform.uppercased())
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                                Spacer()
                                Text(model.onlineDeviceIDs.contains(device.id) ? "Online" : "Offline")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }

                Section("RECENT") {
                    if model.transfers.isEmpty {
                        Text("Nothing sent yet")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.transfers) { transfer in
                            VStack(alignment: .leading, spacing: 5) {
                                Text(transfer.displayName ?? transfer.kind.rawValue.capitalized)
                                    .font(.headline)
                                HStack {
                                    Text(ByteCountFormatter.string(
                                        fromByteCount: transfer.sizeBytes,
                                        countStyle: .file
                                    ))
                                    Spacer()
                                    Text(transfer.status.rawValue)
                                }
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .refreshable { await model.reload() }
            .safeAreaInset(edge: .bottom) {
                Button(action: {}) {
                    Label("SEND", systemImage: "plus")
                        .font(.headline)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 14)
                }
                .buttonStyle(.borderedProminent)
                .padding()
                .background(.ultraThinMaterial)
            }
            .navigationBarHidden(true)
        }
    }

    private var brand: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 8) {
                Text("PIXEL GO")
                    .font(.system(size: 34, weight: .black, design: .rounded))
                Text("Native cross-device sharing.")
                    .foregroundStyle(.secondary)
                if let error = model.errorMessage {
                    Text(error)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
            Button("Sign out") {
                Task { await model.signOut() }
            }
            .font(.caption)
        }
        .padding(.vertical, 24)
    }
}
