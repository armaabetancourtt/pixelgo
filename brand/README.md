# PixelGo / Official visual identity

<p align="center"><img src="pixelgo-banner.svg" width="100%" alt="PixelGo white wordmark on its pastel aurora"/></p>

The approved PixelGo artwork is the visual reference: rounded, custom white lettering with a four-point sparkle over a luminous pastel gradient. The repository SVGs are traced/vector interpretations of the supplied mark, intended to keep its distinctive silhouette across GitHub and product surfaces.

## Canonical assets

| File | Intended use |
| --- | --- |
| [PixelGo wordmark](pixelgo-wordmark.svg) | Transparent white vector wordmark and sparkle, on colorful/dark backgrounds |
| [Pastel banner](pixelgo-banner.svg) | Full-width GitHub README and other editorial headers |
| [App icon](pixelgo-icon.svg) | Compact P + sparkle mark on pastel gradient |
| [Raster wordmark](pixelgo-wordmark.png) | Native iOS/Android branded headers |
| [Raster icon](pixelgo-app-icon.png) | Native iOS/Android launcher |

Do not retype the wordmark, detach its sparkle, modify its silhouette, or stretch it. The white mark needs enough contrast: use the supplied aurora, a saturated violet/blue field, or a dark background; never place transparent white lettering directly over white.

## Palette

| Token | Hex | Role |
| --- | --- | --- |
| Sky | `#8FB9FF` | Foundation of the gradient |
| Violet | `#AD8BFA` | Principal interactive accent |
| Pink | `#FF9DDE` | Warm aurora |
| Aqua | `#64DDF9` | Cool aurora |
| Mint | `#A6F7EF` | Light glow and secondary accent |
| Ivory | `#FFF8F6` | Warm end of the aurora |
| Ink | `#1B2142` | Accessible body text |
| White | `#FFFFFF` | Logo and bright surface |

### Interface rules

- White or near-white working surfaces; keep the aurora in compact product headers and launcher imagery.
- Use the **same gradient family** for iOS SwiftUI and Android Compose brand headers.
- Use violet for active controls and ink for primary text. Do not make every card or input a rainbow gradient.
- Maintain visible focus, readable contrasts, and non-color-only transfer-status distinctions.
- The sparkle is part of the actual mark, not an emoji substitute.

## Brand expression

**Send it. Pick it up anywhere.** PixelGo is a native cross-device sharing project. Its cheerful visual tone does not change its security and transfer guarantees: the docs continue to distinguish implemented features from non-implemented features, and do not claim end-to-end encryption.

## Implementation

The main READMEs, iOS SwiftUI, Android Compose, and native launcher assets refer to the files here. If the official mark changes later, regenerate native PNGs from the approved artwork and keep this folder as the single visual source of truth.
