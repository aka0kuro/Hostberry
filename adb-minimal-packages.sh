#!/usr/bin/env bash
#
# Script MUY CONSERVADOR: solo deshabilita una lista negra de paquetes
# claramente opcionales. Todo lo demás NO SE TOCA → evita bootloop.
#
# Reversible: adb shell pm enable <package>
# Si hay bootloop: adb-recovery-bootloop.sh
#
# Uso: ./adb-minimal-packages.sh
#

set -uo pipefail

if ! command -v adb &>/dev/null; then
    echo "Error: adb no está instalado."
    exit 1
fi
if ! adb devices | grep -q 'device$'; then
    echo "Error: No hay dispositivo conectado (depuración USB)."
    exit 1
fi

# LISTA NEGRA: solo estos se deshabilitan. Resto del sistema no se toca.
# Solo apps opcionales (demo, entretenimiento, accesibilidad extra, etc.)
DISABLE_LIST=(
    com.google.android.retaildemo
    com.google.android.apps.retaildemo.preload
    com.google.android.apps.tips
    com.android.egg
    com.google.android.apps.youtube.music
    com.google.android.videos
    com.google.android.apps.photos
    com.google.android.apps.maps
    com.google.android.apps.docs
    com.google.android.calculator
    com.google.android.deskclock
    com.google.android.gm
    com.google.android.apps.tachyon
    com.google.android.youtube
    com.google.android.GoogleCamera
    com.android.chrome
    com.google.android.marvin.talkback
    com.google.android.accessibility.soundamplifier
    com.google.android.accessibility.switchaccess
    com.google.android.apps.accessibility.voiceaccess
    com.google.audio.hearing.visualization.accessibility.scribe
    com.google.android.apps.wellbeing
    com.google.android.apps.emojiwallpaper
    com.google.android.apps.aiwallpapers
    com.google.android.apps.wallpaper.pixel
    com.google.pixel.livewallpaper
    com.google.android.wallpaper.effects
    com.google.android.apps.wallpaper
    com.google.android.feedback
    com.google.android.apps.internal.betterbug
    com.google.android.apps.recorder
    com.google.android.apps.restore
    com.google.android.apps.weather
    com.google.android.apps.pixel.creativeassistant
    com.google.android.apps.pixel.nowplaying
    com.google.android.dreamlinerupdater
    com.google.android.apps.dreamliner
    com.google.android.apps.pixel.dcservice
    com.google.android.apps.pixel.relationships
    com.google.android.apps.pixel.support
    com.google.android.apps.pixel.health
    com.google.android.apps.customization.pixel
    com.google.android.pixel.avatarpicker
    com.google.android.apps.magicportrait
    com.google.android.apps.scone
    com.google.android.apps.diagnosticstool
    com.google.android.apps.safetyhub
    com.google.android.apps.wearables.maestro.companion
    com.google.android.apps.carrier.log
    com.google.android.apps.cbrsnetworkmonitor
    com.google.android.apps.work.oobconfig
    com.google.android.apps.work.clouddpc
    com.google.android.apps.nbu.files
    com.google.android.calendar
    com.android.providers.calendar
    com.google.android.printservice.recommendation
    com.google.android.hardwareinfo
    com.google.assistant.hubui
    com.google.android.hotspot2.osulogin
    com.google.android.flipendo
    com.google.android.ar.core
    com.google.android.settings.intelligence
    com.google.SSRestartDetector
    com.google.android.health.connect.backuprestore
    com.google.android.healthconnect.controller
    com.android.healthconnect.overlay
    com.google.android.apps.privacy.wildlife
    com.google.android.uvexposurereporter
    com.google.android.wfcactivation
    com.google.android.repairmode
    com.google.android.configupdater
    com.google.android.modulemetadata
    com.google.android.turboadapter
    com.google.android.avatarpicker
    com.google.android.googlequicksearchbox
    com.google.android.googlequicksearchbox.nga_resources
    com.google.android.tts
    com.google.android.markup
    com.google.android.partnersetup
    com.google.android.odad
    com.google.android.verifier
    com.google.android.verifier.overlay
    com.google.android.federatedcompute
    com.google.android.ondevicepersonalization.services
    com.google.android.appsearch.apk
    com.google.android.sdksandbox
    com.google.android.webview
    com.google.android.inputmethod.latin
)

echo "=== ADB Minimal (SOLO lista negra - muy conservador) ==="
echo "Solo se deshabilitan ${#DISABLE_LIST[@]} paquetes opcionales."
echo "Todo lo demás (system ui, permisos, overlays, etc.) NO se toca."
echo ""
read -p "¿Continuar? (s/N): " -r
[[ "${REPLY,,}" == "s" || "${REPLY,,}" == "si" ]] || exit 0

OK=0
FAIL=0
for pkg in "${DISABLE_LIST[@]}"; do
    if adb shell pm disable-user --user 0 "$pkg" 2>/dev/null; then
        echo "[DESHABILITADO] $pkg"
        ((OK++)) || true
    else
        echo "[--] $pkg (no instalado o no deshabilitable)"
        ((FAIL++)) || true
    fi
done

echo ""
echo "=== Resumen ==="
echo "Deshabilitados: $OK"
echo "No tocados/fallo: $FAIL"
echo ""
echo "Reinicia si quieres. Revertir: adb shell pm enable <package>"
echo "Si bootloop: ./adb-recovery-bootloop.sh"
