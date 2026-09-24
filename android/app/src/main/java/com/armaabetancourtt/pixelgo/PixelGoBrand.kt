package com.armaabetancourtt.pixelgo

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** Shared pastel tokens, aligned with brand/README.md and the iOS client. */
internal object PixelGoPalette {
    val sky = Color(0xFF8FB9FF)
    val violet = Color(0xFFAD8BFA)
    val pink = Color(0xFFFF9DDE)
    val aqua = Color(0xFF64DDF9)
    val mint = Color(0xFFA6F7EF)
    val ink = Color(0xFF1B2142)
    val canvas = Color(0xFFFFFCFF)
}

internal val pixelGoColorScheme = lightColorScheme(
    primary = Color(0xFF7955D0),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFECE3FF),
    onPrimaryContainer = PixelGoPalette.ink,
    secondary = Color(0xFF356F91),
    onSecondary = Color.White,
    background = PixelGoPalette.canvas,
    onBackground = PixelGoPalette.ink,
    surface = Color.White,
    onSurface = PixelGoPalette.ink,
    surfaceVariant = Color(0xFFF7F1FF),
    onSurfaceVariant = Color(0xFF575C75),
    outline = Color(0xFFB5B0CD)
)

/**
 * The approved PixelGo white custom wordmark is a drawable, not a font.
 * The composable is intentionally presentational: no transfer behavior changes.
 */
@Composable
internal fun PixelGoBrandHeader(compact: Boolean = false) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(if (compact) 138.dp else 202.dp)
            .clip(RoundedCornerShape(24.dp))
            .background(
                Brush.linearGradient(
                    colors = listOf(
                        PixelGoPalette.sky,
                        PixelGoPalette.violet,
                        PixelGoPalette.pink,
                        PixelGoPalette.aqua
                    )
                )
            )
            .padding(if (compact) 18.dp else 24.dp)
    ) {
        Image(
            painter = painterResource(id = R.drawable.pixelgo_wordmark),
            contentDescription = "PixelGo",
            modifier = Modifier
                .align(Alignment.Center)
                .fillMaxWidth()
                .height(if (compact) 76.dp else 112.dp),
            contentScale = ContentScale.Fit
        )
        Text(
            text = "SEND IT. PICK IT UP ANYWHERE.",
            modifier = Modifier.align(Alignment.BottomStart),
            fontSize = if (compact) 9.sp else 11.sp,
            letterSpacing = 1.2.sp,
            fontWeight = FontWeight.Bold,
            color = Color.White
        )
    }
}
