import SwiftUI

/// PixelGo's shared pastel identity. The wordmark is the raster asset derived
/// from the approved logo; never substitute a generic Text rendering for it.
enum PixelGoBrand {
    static let ink = Color(red: 27.0 / 255.0, green: 33.0 / 255.0, blue: 66.0 / 255.0)
    static let sky = Color(red: 143.0 / 255.0, green: 185.0 / 255.0, blue: 255.0 / 255.0)
    static let violet = Color(red: 173.0 / 255.0, green: 139.0 / 255.0, blue: 250.0 / 255.0)
    static let pink = Color(red: 255.0 / 255.0, green: 157.0 / 255.0, blue: 222.0 / 255.0)
    static let aqua = Color(red: 100.0 / 255.0, green: 221.0 / 255.0, blue: 249.0 / 255.0)
    static let mint = Color(red: 166.0 / 255.0, green: 247.0 / 255.0, blue: 239.0 / 255.0)
    static let canvas = Color(red: 255.0 / 255.0, green: 252.0 / 255.0, blue: 255.0 / 255.0)
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
