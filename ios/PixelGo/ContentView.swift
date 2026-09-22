import PhotosUI
import SwiftUI
import UIKit
import UniformTypeIdentifiers

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

            Button(
                createAccount
                    ? "Already have an account? Sign in"
                    : "New to PIXEL GO? Create account"
            ) {
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
    @State private var showingSend = false
    @State private var showingExporter = false
    @State private var exportDocument = PayloadDocument(data: Data())
    @State private var exportFilename = "PIXEL-GO-File"
    @State private var exportTransferID: String?
    @State private var exportTemporaryURL: URL?

    private var destinationDevices: [PixelDevice] {
        model.devices.filter { $0.id != model.localDeviceID }
    }

    private var readyBinaryTransfers: [Transfer] {
        model.transfers.filter {
            $0.destinationDeviceId == model.localDeviceID &&
            $0.status == .ready &&
            ($0.kind == .file || $0.kind == .photo)
        }
    }

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
                                    .fill(
                                        model.onlineDeviceIDs.contains(device.id)
                                            ? Color.green
                                            : Color.gray
                                    )
                                    .frame(width: 8, height: 8)

                                VStack(alignment: .leading, spacing: 2) {
                                    HStack(spacing: 6) {
                                        Text(device.name)
                                            .font(.headline)
                                        if device.id == model.localDeviceID {
                                            Text("THIS DEVICE")
                                                .font(.system(size: 9, weight: .bold))
                                                .foregroundStyle(.secondary)
                                        }
                                    }

                                    Text(device.platform.uppercased())
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }

                                Spacer()

                                Text(
                                    model.onlineDeviceIDs.contains(device.id)
                                        ? "Online"
                                        : "Offline"
                                )
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            }
                        }
                    }
                }

                if !model.receivedItems.isEmpty {
                    Section("INBOX") {
                        ForEach(model.receivedItems) { item in
                            VStack(alignment: .leading, spacing: 5) {
                                Text(inboxLabel(for: item.kind))
                                    .font(.caption2)
                                    .fontWeight(.bold)
                                    .foregroundStyle(.secondary)
                                Text(item.text)
                                    .font(.body)
                                    .textSelection(.enabled)
                                    .lineLimit(4)
                            }
                            .padding(.vertical, 2)
                        }
                    }
                }

                if !readyBinaryTransfers.isEmpty {
                    Section("READY TO SAVE") {
                        ForEach(readyBinaryTransfers) { transfer in
                            HStack {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(
                                        transfer.displayName
                                            ?? (transfer.kind == .photo ? "Photo" : "File")
                                    )
                                    .font(.headline)

                                    Text(
                                        ByteCountFormatter.string(
                                            fromByteCount: transfer.sizeBytes,
                                            countStyle: .file
                                        )
                                    )
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                }

                                Spacer()

                                Button("Save") {
                                    Task {
                                        guard let fileURL = await model.downloadPayloadFile(
                                            for: transfer
                                        ) else {
                                            return
                                        }

                                        exportDocument = PayloadDocument(fileURL: fileURL)
                                        exportTemporaryURL = fileURL
                                        exportFilename = transfer.displayName
                                            ?? (transfer.kind == .photo
                                                ? "PIXEL-GO-Photo"
                                                : "PIXEL-GO-File")
                                        exportTransferID = transfer.id
                                        showingExporter = true
                                    }
                                }
                                .disabled(model.isLoading)
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
                                Text(
                                    transfer.displayName
                                        ?? transfer.kind.rawValue.capitalized
                                )
                                .font(.headline)

                                HStack {
                                    Text(
                                        ByteCountFormatter.string(
                                            fromByteCount: transfer.sizeBytes,
                                            countStyle: .file
                                        )
                                    )
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
                Button {
                    showingSend = true
                } label: {
                    Label("SEND", systemImage: "plus")
                        .font(.headline)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 14)
                }
                .buttonStyle(.borderedProminent)
                .disabled(destinationDevices.isEmpty || model.isLoading)
                .padding()
                .background(.ultraThinMaterial)
            }
            .sheet(isPresented: $showingSend) {
                SendView(destinations: destinationDevices)
                    .environmentObject(model)
            }
            .fileExporter(
                isPresented: $showingExporter,
                document: exportDocument,
                contentType: .data,
                defaultFilename: exportFilename
            ) { result in
                let temporaryURL = exportTemporaryURL

                switch result {
                case .success:
                    guard let transferID = exportTransferID else { return }
                    Task {
                        await model.completeIncomingTransfer(transferID)
                        if let temporaryURL {
                            try? FileManager.default.removeItem(at: temporaryURL)
                        }
                        exportTemporaryURL = nil
                        exportTransferID = nil
                    }
                case .failure(let error):
                    if let temporaryURL {
                        try? FileManager.default.removeItem(at: temporaryURL)
                    }
                    exportTemporaryURL = nil
                    model.errorMessage = error.localizedDescription
                    exportTransferID = nil
                }
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

                if destinationDevices.isEmpty {
                    Text("Sign in on another device to start sending.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

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

private struct SendView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss

    let destinations: [PixelDevice]

    @State private var selectedDeviceID: String
    @State private var text = ""
    @State private var selectedPhoto: PhotosPickerItem?
    @State private var showingFileImporter = false

    init(destinations: [PixelDevice]) {
        self.destinations = destinations
        _selectedDeviceID = State(
            initialValue: destinations.first?.id ?? ""
        )
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("TO") {
                    Picker("Device", selection: $selectedDeviceID) {
                        ForEach(destinations) { device in
                            Text(device.name).tag(device.id)
                        }
                    }
                }

                Section("TEXT OR LINK") {
                    TextEditor(text: $text)
                        .frame(minHeight: 120)

                    Button("Send text / link") {
                        Task {
                            if await model.sendText(text, to: selectedDeviceID) {
                                dismiss()
                            }
                        }
                    }
                    .disabled(
                        selectedDeviceID.isEmpty ||
                        text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                        model.isLoading
                    )
                }

                Section("CLIPBOARD") {
                    Button {
                        Task { await sendClipboard() }
                    } label: {
                        Label("Paste & send clipboard", systemImage: "doc.on.clipboard")
                    }
                    .disabled(selectedDeviceID.isEmpty || model.isLoading)

                    Text("Clipboard text is sent as its own transfer type using the same signed delivery pipeline.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Section("PHOTO OR FILE") {
                    PhotosPicker(
                        selection: $selectedPhoto,
                        matching: .images
                    ) {
                        Label("Choose photo", systemImage: "photo")
                    }
                    .disabled(selectedDeviceID.isEmpty || model.isLoading)

                    Button {
                        showingFileImporter = true
                    } label: {
                        Label("Choose file", systemImage: "doc")
                    }
                    .disabled(selectedDeviceID.isEmpty || model.isLoading)

                    Text("Payload bytes upload directly to object storage through a short-lived signed URL.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                if model.isLoading {
                    Section {
                        HStack {
                            Spacer()
                            ProgressView("Sending…")
                            Spacer()
                        }
                    }
                }
            }
            .navigationTitle("Send")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
            .onChange(of: selectedPhoto) { _, item in
                guard let item else { return }
                Task { await sendPhoto(item) }
            }
            .fileImporter(
                isPresented: $showingFileImporter,
                allowedContentTypes: [.item],
                allowsMultipleSelection: false
            ) { result in
                guard case .success(let urls) = result, let url = urls.first else {
                    return
                }
                Task { await sendFile(url) }
            }
        }
    }

    private func sendClipboard() async {
        guard
            let value = UIPasteboard.general.string?
                .trimmingCharacters(in: .whitespacesAndNewlines),
            !value.isEmpty
        else {
            model.errorMessage = "Clipboard does not contain text."
            return
        }

        if await model.sendPayload(
            Data(value.utf8),
            kind: .clipboard,
            displayName: "Clipboard",
            contentType: "text/plain; charset=utf-8",
            to: selectedDeviceID
        ) {
            dismiss()
        }
    }

    private func sendPhoto(_ item: PhotosPickerItem) async {
        do {
            guard let data = try await item.loadTransferable(type: Data.self) else {
                return
            }

            let type = item.supportedContentTypes.first ?? .image
            let ext = type.preferredFilenameExtension ?? "jpg"
            let mime = type.preferredMIMEType ?? "image/jpeg"

            if await model.sendPayload(
                data,
                kind: .photo,
                displayName: "Photo.\(ext)",
                contentType: mime,
                to: selectedDeviceID
            ) {
                dismiss()
            }
        } catch {
            model.errorMessage = error.localizedDescription
        }
    }

    private func sendFile(_ url: URL) async {
        let scoped = url.startAccessingSecurityScopedResource()
        defer {
            if scoped {
                url.stopAccessingSecurityScopedResource()
            }
        }

        do {
            let values = try? url.resourceValues(forKeys: [.contentTypeKey])
            let mime = values?.contentType?.preferredMIMEType
                ?? "application/octet-stream"

            if await model.sendFile(
                at: url,
                kind: .file,
                displayName: url.lastPathComponent,
                contentType: mime,
                to: selectedDeviceID
            ) {
                dismiss()
            }
        } catch {
            model.errorMessage = error.localizedDescription
        }
    }
}


private func inboxLabel(for kind: Transfer.Kind) -> String {
    switch kind {
    case .link:
        return "LINK"
    case .clipboard:
        return "CLIPBOARD"
    default:
        return "TEXT"
    }
}


private struct PayloadDocument: FileDocument {
    static var readableContentTypes: [UTType] { [.data] }

    private enum Source {
        case data(Data)
        case file(URL)
    }

    private var source: Source

    init(data: Data) {
        source = .data(data)
    }

    init(fileURL: URL) {
        source = .file(fileURL)
    }

    init(configuration: ReadConfiguration) throws {
        source = .data(configuration.file.regularFileContents ?? Data())
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        switch source {
        case .data(let data):
            return FileWrapper(regularFileWithContents: data)
        case .file(let fileURL):
            return try FileWrapper(url: fileURL, options: [])
        }
    }
}
