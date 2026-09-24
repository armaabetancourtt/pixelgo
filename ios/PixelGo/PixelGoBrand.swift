import SwiftUI

/// PixelGo's shared pastel identity. The wordmark is the raster asset derived
/// from the approved logo; never substitute a generic Text rendering for it.
enum PixelGoBrand {
    static let ink = Color(red: 27 / 255, green: 33 / 255, blue: 66 / 255)
    static let sky = Color(red: 143 / 255, green: 185 / 255, blue: 255 / 255)
    static let violet = Color(red: 173 / 255, green: 139 / 255, blue: 250 / 255)
    static let pink = Color(red: 255 / 255, green: 157 / 255, blue: 222 / 255)
    static let aqua = Color(red: 100 / 255, green: 221 / 255, blue: 249 / 255)
    static let mint = Color(red: 166 / 255, green: 247 / 255, blue: 239 / 255)
    static let canvas = Color(red: 255 / 255, green: 252 / 255, blue: 255 / 255)
}

struct PixelGoBrandHeader: View {
    var compact = false

    var body: some View {
        ZStack(alignment: .bottomLeading) {
            RoundedRectangle(cornerRadius: 24, style: .continuous)
                .fill(
                    LinearGradient(
                        colors: [
                            PixelGoBrand.sky,
                            PixelGoBrand.violet,
                            PixelGoBrand.pink,
                            PixelGoBrand.aqua
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )

            Circle()
                .fill(PixelGoBrand.mint.opacity(0.52))
                .frame(width: compact ? 160 : 230, height: compact ? 160 : 230)
                .blur(radius: 48)
                .offset(x: compact ? 175 : 235, y: compact ? 55 : 95)

            VStack(alignment: .leading, spacing: compact ? 6 : 16) {
                Spacer(minLength: 0)
                Image("PixelGoWordmark")
                    .resizable()
                    .scaledToFit()
                    .frame(maxWidth: compact ? 230 : 320, alignment: .leading)
                    .accessibilityLabel("PixelGo")
                Text("SEND IT. PICK IT UP ANYWHERE.")
                    .font(.system(size: compact ? 9 : 11, weight: .bold, design: .rounded))
                    .tracking(1.2)
                    .foregroundStyle(.white)
            }
            .padding(compact ? 18 : 24)
        }
        .frame(height: compact ? 138 : 202)
        .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
        .accessibilityElement(children: .combine)
    }
}
