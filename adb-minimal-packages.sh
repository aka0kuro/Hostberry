#!/usr/bin/env bash
#
# Script para dejar en el dispositivo Android solo:
# - Wallet y lo relacionado (NFC, credential manager, secure element)
# - System UI
# - Bluetooth, WiFi, WiFi tethering
# - NFC
# - Play Store
# - Pixel Launcher (Nexus Launcher)
# - Servicios mínimos necesarios para que el sistema arranque
#
# Uso: conecta el dispositivo por USB con depuración ADB y ejecuta:
#   ./adb-minimal-packages.sh
#
# IMPORTANTE: Usa "pm disable-user" (reversible). Para desinstalar de verdad
# tendrías que usar "pm uninstall" y muchas apps de sistema no lo permiten sin root.
# Haz backup antes. Puede requerir reinicio.
#

set -uo pipefail

# Comprobar que ADB está disponible y hay dispositivo
if ! command -v adb &>/dev/null; then
    echo "Error: adb no está instalado. Instala android-tools-adb."
    exit 1
fi
if ! adb devices | grep -q 'device$'; then
    echo "Error: No hay ningún dispositivo Android conectado (USB depuración activada)."
    exit 1
fi

# Paquetes que SÍ queremos mantener (whitelist)
KEEP=(
    # Sistema base
    android
    com.android.settings
    com.android.phone
    com.android.systemui
    com.android.keychain
    com.android.se
    com.android.credentialmanager
    com.android.inputdevices
    com.android.shell
    com.android.multiuser
    com.android.proxyhandler
    com.android.localtransport
    com.android.sharedstoragebackup
    com.android.externalstorage
    com.android.backupconfirm
    com.android.emergency
    com.android.vpndialogs
    com.android.managedprovisioning
    com.android.devicelockcontroller
    com.android.mtp
    com.android.intentresolver
    com.android.soundpicker
    com.android.wallpaperbackup
    com.android.wallpaper.livepicker
    com.android.dreams.basic
    com.android.certinstaller
    com.android.htmlviewer
    com.android.bips
    com.android.printspooler
    # Providers esenciales
    com.android.providers.settings
    com.android.providers.telephony
    com.android.providers.contacts
    com.android.providers.media
    com.android.providers.downloads
    com.android.providers.blockednumber
    com.android.providers.userdictionary
    com.android.providers.partnerbookmarks
    com.android.providers.downloads.ui
    com.android.bookmarkprovider
    com.android.server.telecom
    # Wallet, NFC, pagos
    com.android.systemui.plugin.globalactions.wallet
    com.google.android.nfc
    com.android.nfc
    com.google.android.pixelnfc
    com.google.android.tag
    com.google.android.euicc
    com.google.euiccpixel
    # Bluetooth
    com.google.android.bluetooth
    com.android.bluetoothmidiservice
    # WiFi y red / tethering
    com.google.android.networkstack
    com.google.android.networkstack.tethering
    com.android.networkstack.overlay
    com.google.android.wifi.dialog
    com.google.android.wifi.resources
    com.google.android.wifi.resources.pixel
    com.google.android.captiveportallogin
    com.android.captiveportallogin.overlay
    com.google.android.connectivity.resources
    com.google.android.connectivity.resources.overlay
    com.google.android.networkstack.tethering
    # Play Store y servicios Google mínimos
    com.android.vending
    com.google.android.gms
    com.google.android.gsf
    com.google.android.ext.services
    com.google.android.ext.shared
    com.google.android.packageinstaller
    com.google.android.permissioncontroller
    # Launcher
    com.google.android.apps.nexuslauncher
    # Teclado y contacto básico
    com.google.android.inputmethod.latin
    com.google.android.contacts
    # Telefonía mínima
    com.android.mms.service
    com.android.carrierdefaultapp
    com.android.cellbroadcastreceiver
    com.android.stk
    com.android.simappdialog
    com.android.telephony.imsmedia
    com.android.ons
    com.android.qns
    # Documentos / archivos básico
    com.google.android.documentsui
    # Overlays mínimos para que System UI y Settings no rompan (genéricos)
    com.android.systemui.auto_generated_rro_product__
    com.android.settings.auto_generated_rro_product__
    android.auto_generated_rro_product__
    android.auto_generated_characteristics_rro
    android.auto_generated_rro_vendor__
    com.android.phone.auto_generated_rro_product__
    com.android.providers.settings.auto_generated_rro_product__
    com.android.providers.contacts.auto_generated_rro_product__
    com.android.providers.telephony.auto_generated_rro_product__
    com.android.server.telecom.auto_generated_rro_product__
)

# Lista completa de paquetes del dispositivo (la que proporcionaste)
ALL_PACKAGES=(
    package:com.google.audio.hearing.visualization.accessibility.scribe
    package:com.android.companiondevicemanager.auto_generated_characteristics_rro
    package:com.android.systemui.auto_generated_rro_vendor__
    package:com.google.android.providers.media.module
    package:com.google.android.retaildemo
    package:com.google.android.overlay.permissioncontroller
    package:com.google.android.overlay.googlewebview
    package:com.android.devicediagnostics.auto_generated_rro_vendor__
    package:com.google.android.pixel.avatarpicker
    package:com.android.calllogbackup
    package:com.google.android.overlay.glanceablehubconfig
    package:com.android.omadm.service
    package:com.google.android.microdroid.empty_payload
    package:com.android.systemui.accessibility.accessibilitymenu
    package:com.google.android.nfc
    package:com.android.providers.contacts
    package:com.google.android.apps.nbu.files
    package:com.android.dreams.basic
    package:com.android.companiondevicemanager
    package:com.android.cts.priv.ctsshim
    package:com.google.android.calendar
    package:com.google.android.contacts
    package:com.android.mms.service
    package:com.google.android.cellbroadcastreceiver
    package:com.android.providers.downloads
    package:com.android.bluetoothmidiservice
    package:com.android.credentialmanager
    package:com.google.android.apps.wallpaper.pixel
    package:com.google.android.photopicker
    package:com.android.networkstack.overlay
    package:com.google.android.apps.pixel.customizationbundle
    package:com.google.android.euiccoverlay
    package:com.google.android.printservice.recommendation
    package:com.google.android.captiveportallogin
    package:com.google.android.overlay.glanceablehubsettings
    package:com.google.android.networkstack
    package:com.android.captiveportallogin.overlay
    package:com.google.android.telephony
    package:com.google.android.overlay.googleconfig
    package:com.google.android.apps.pixel.health
    package:com.android.keychain
    package:com.google.android.hardwareinfo
    package:com.google.android.accessibility.switchaccess
    package:com.google.android.tag
    package:android.auto_generated_rro_vendor__
    package:com.android.devicediagnostics.auto_generated_rro_product__
    package:com.google.android.dreamlinerupdater
    package:com.google.android.apps.wellbeing
    package:com.google.android.apps.emojiwallpaper
    package:com.android.healthconnect.overlay
    package:com.android.shell
    package:com.google.android.adservices.api
    package:com.google.android.wifi.dialog
    package:com.google.android.systemui.overlay.pixelvpnconfig
    package:com.android.inputdevices
    package:com.google.android.networkstack.tethering.overlay2021
    package:com.google.android.appsearch.apk
    package:com.google.android.ondevicepersonalization.services
    package:com.google.euiccpixel
    package:com.google.android.apps.customization.pixel
    package:com.android.bookmarkprovider
    package:com.google.android.rilextension
    package:com.google.android.apps.tips
    package:com.android.settings.husky
    package:com.google.android.onetimeinitializer
    package:com.google.android.permissioncontroller
    package:com.android.DeviceAsWebcam
    package:com.android.sharedstoragebackup
    package:com.google.android.apps.cbrsnetworkmonitor
    package:com.android.imsserviceentitlement
    package:com.android.providers.media
    package:com.android.providers.calendar
    package:com.android.settings.overlay.husky
    package:com.google.assistant.hubui
    package:com.android.providers.blockednumber
    package:com.google.android.documentsui
    package:com.android.multiuser
    package:com.google.android.apps.internal.betterbug
    package:com.google.android.verifier.overlay
    package:com.android.systemui.clocks.metro
    package:com.android.proxyhandler
    package:com.android.managedprovisioning
    package:com.android.emergency
    package:com.google.android.gms.location.history
    package:com.google.android.uwb.resources.pixel
    package:com.google.android.apps.aiwallpapers
    package:com.android.omadm.service.auto_generated_rro_vendor__
    package:com.google.android.storagemanager.auto_generated_rro_vendor__
    package:com.google.android.gm
    package:com.android.carrierdefaultapp
    package:com.android.backupconfirm
    package:com.google.android.apps.tachyon
    package:com.google.android.flipendo
    package:com.google.android.hotspot2.osulogin
    package:com.android.nfc
    package:com.google.android.deskclock
    package:com.android.mtp
    package:com.google.android.gsf
    package:com.google.android.apps.accessibility.voiceaccess
    package:com.google.android.overlay.pixelconfigcommon
    package:com.google.android.apps.privacy.wildlife
    package:com.google.android.carrierlocation
    package:com.android.phone.auto_generated_characteristics_rro
    package:com.google.pixel.livewallpaper
    package:com.android.internal.display.cutout.emulation.double
    package:com.android.theme.font.notoserifsource
    package:com.google.android.wallpaper.effects
    package:com.android.pixeldisplayservice.auto_generated_rro_product__
    package:com.android.traceur.auto_generated_rro_product__
    package:com.google.android.health.connect.backuprestore
    package:com.android.cellbroadcastreceiver.overlay.pixel
    package:com.android.systemui.clocks.bignum
    package:com.google.SSRestartDetector
    package:com.google.android.settings.intelligence
    package:android.autoinstalls.config.google.nexus
    package:com.android.systemui.clocks.weather
    package:com.android.managedprovisioning.overlay
    package:com.android.systemui
    package:com.google.ar.core
    package:com.vzw.apnlib
    package:com.android.providers.contacts.auto_generated_rro_product__
    package:com.google.android.dialer
    package:com.google.android.flipendo.auto_generated_rro_product__
    package:com.samsung.slsi.telephony.oem.oemrilhookservice
    package:com.google.android.grilservice
    package:com.verizon.mips.services
    package:com.android.internal.systemui.navbar.gestural
    package:com.android.role.notes.enabled
    package:com.google.android.apps.nexuslauncher
    package:com.google.android.overlay.pixelconfig2021
    package:com.google.android.settings.overlay.pixelvpnconfig
    package:com.shannon.imsservice
    package:com.google.mainline.adservices
    package:com.google.android.calculator
    package:com.android.devicediagnostics
    package:com.android.internal.display.cutout.emulation.avoidAppsInCutout
    package:com.android.hotwordenrollment.okgoogle
    package:com.google.euiccpixel.overlay.zuma
    package:com.android.qns
    package:com.google.android.factoryota
    package:com.google.android.apps.wallpaper
    package:com.google.android.federatedcompute
    package:com.android.systemui.clocks.numoverlap
    package:com.google.android.webview
    package:com.google.android.sdksandbox
    package:com.google.android.storagemanager
    package:com.android.wallpaperbackup
    package:com.google.android.cellbroadcastservice
    package:com.android.sdm.plugins.diagmon
    package:com.verizon.services
    package:com.google.android.apps.pixel.creativeassistant
    package:com.android.internal.systemui.navbar.threebutton
    package:com.android.egg
    package:com.android.localtransport
    package:android
    package:com.google.android.pixelsystemservice
    package:com.google.android.overlay.pixelconfig2018
    package:com.google.android.virtualmachine.res
    package:com.google.android.pixelnfc
    package:com.google.android.pixel.setupwizard.overlay.expressive
    package:com.android.providers.settings.auto_generated_rro_product__
    package:com.android.hotwordenrollment.xgoogle
    package:com.google.android.soundpicker
    package:com.android.settings.auto_generated_rro_vendor__
    package:com.android.internal.display.cutout.emulation.noCutout
    package:com.google.android.packageinstaller
    package:com.android.se
    package:com.android.pacprocessor
    package:com.android.providers.media.overlay.pixel
    package:com.google.android.tetheringentitlement
    package:com.google.android.safetycenter.resources
    package:com.google.android.settings.future.biometrics.faceenroll
    package:com.google.android.apps.youtube.music
    package:com.google.android.overlay.permissioncontroller.safetycenter
    package:com.android.stk
    package:com.google.pixel.digitalkey.timesync
    package:com.android.internal.display.cutout.emulation.hole
    package:com.google.android.systemui.overlay.glanceablehubconfig
    package:com.android.settings
    package:com.android.bips
    package:com.google.android.partnersetup
    package:com.android.internal.display.cutout.emulation.tall
    package:com.google.android.networkstack.tethering
    package:com.android.sdm.plugins.connmo
    package:com.google.android.projection.gearhead
    package:com.android.cameraextensions
    package:com.android.safetyregulatoryinfo
    package:com.google.android.odad
    package:com.google.android.apps.carrier.carrierwifi
    package:com.android.traceur.auto_generated_rro_vendor__
    package:com.shannon.rcsservice
    package:com.google.android.videos
    package:com.google.android.ext.shared
    package:com.google.android.apps.pixel.nowplaying
    package:com.google.android.feedback
    package:com.android.chrome
    package:com.google.android.overlay.trafficlightfaceoverlay
    package:com.google.android.apps.maps
    package:com.google.android.apps.camera.services
    package:com.google.android.wifi.resources.pixel
    package:com.android.devicelockcontroller
    package:com.google.android.as
    package:android.auto_generated_rro_product__
    package:com.android.musicfx
    package:android.auto_generated_characteristics_rro
    package:com.android.internal.systemui.navbar.transparent
    package:com.android.server.telecom.auto_generated_rro_product__
    package:com.google.android.inputmethod.latin
    package:com.google.android.accessibility.soundamplifier
    package:com.google.android.apps.carrier.log
    package:com.google.android.apps.weather
    package:com.google.android.overlay.udfpsoverlay
    package:com.google.android.marvin.talkback
    package:com.android.uwb.resources.pixel
    package:com.google.android.systemui.gxoverlay
    package:com.google.android.uwb.resources
    package:com.android.providers.downloads.ui
    package:com.google.android.wifi.resources
    package:com.android.ons
    package:com.google.android.GoogleCamera
    package:com.google.android.healthconnect.controller
    package:com.android.intentresolver
    package:com.google.android.apps.docs
    package:com.android.phone.auto_generated_rro_vendor__
    package:com.android.certinstaller
    package:com.google.android.setupwizard
    package:com.google.android.apps.safetyhub
    package:com.google.android.apps.retaildemo.preload
    package:com.google.android.apps.recorder
    package:com.google.android.apps.restore
    package:com.google.android.systemui.overlay.pixelbatteryhealthconfig
    package:com.android.systemui.clocks.growth
    package:com.android.simappdialog
    package:com.android.providers.telephony
    package:com.android.wallpaper.livepicker
    package:com.google.android.carriersetup
    package:com.android.systemui.clocks.calligraphy
    package:com.google.android.apps.pixel.dcservice
    package:com.google.android.pcs
    package:com.google.android.uvexposurereporter
    package:com.google.android.connectivity.resources.overlay
    package:com.android.internal.display.cutout.emulation.waterfall
    package:com.android.settings.auto_generated_rro_product__
    package:com.google.android.rkpdapp
    package:com.google.android.nfc.overlay
    package:com.android.providers.settings
    package:com.google.android.pixel.setupwizard
    package:com.android.phone
    package:com.google.android.overlay.pixelconfig2019midyear
    package:com.google.android.flipendo.auto_generated_rro_product__
    package:com.google.android.apps.work.oobconfig
    package:com.android.traceur
    package:com.android.systemui.clocks.inflate
    package:com.google.android.as.oss
    package:com.google.android.apps.messaging
    package:com.google.android.apps.diagnosticstool
    package:com.google.android.repairmode
    package:com.google.android.apps.wearables.maestro.companion
    package:com.android.location.fused
    package:com.android.vpndialogs
    package:com.samsung.slsi.telephony.oem.oemril
    package:com.android.cellbroadcastreceiver
    package:com.android.systemui.plugin.globalactions.wallet
    package:com.google.android.tts
    package:com.google.android.googlequicksearchbox
    package:com.google.android.turboadapter
    package:com.google.android.avatarpicker
    package:com.google.android.modulemetadata
    package:com.google.RilConfigService
    package:com.android.phone.auto_generated_rro_product__
    package:com.google.android.documentsui.theme.pixel
    package:com.google.android.apps.work.clouddpc
    package:com.android.systemui.accessibility.accessibilitymenu.auto_generated_rro_product__
    package:com.android.htmlviewer
    package:com.android.vending
    package:com.google.omadm.trigger
    package:com.android.omadm.service.auto_generated_rro_product__
    package:com.google.euiccpixel.permissions
    package:com.google.android.ext.services
    package:com.google.android.configupdater
    package:com.android.sdm.plugins.dcmo
    package:com.google.android.apps.pixel.relationships
    package:com.google.android.apps.turbo
    package:com.google.android.compos.payload
    package:com.google.android.aicore
    package:com.google.android.nfc.overlay.common
    package:com.google.android.gms.supervision
    package:com.android.virtualization.terminal
    package:com.android.providers.userdictionary
    package:com.google.android.apps.magicportrait
    package:com.android.providers.contactkeys
    package:com.google.android.server.deviceconfig.resources
    package:com.android.cts.ctsshim
    package:com.google.android.apps.photos
    package:com.android.cellbroadcastservice.overlay.pixel
    package:com.google.android.markup
    package:com.google.android.apps.scone
    package:com.google.android.wfcactivation
    package:com.google.android.connectivitythermalpowermanager
    package:com.android.internal.display.cutout.emulation.corner
    package:com.google.android.gms
    package:com.google.android.verifier
    package:com.android.printspooler
    package:com.android.pixeldisplayservice
    package:com.android.systemui.auto_generated_rro_product__
    package:com.google.android.apps.dreamliner
    package:com.google.android.storagemanager.auto_generated_rro_product__
    package:com.google.android.apps.setupwizard.searchselector
    package:com.android.providers.partnerbookmarks
    package:com.android.soundpicker
    package:com.google.pixel.camera.services
    package:com.google.mainline.telemetry
    package:com.google.android.apps.pixel.support
    package:com.google.ambient.streaming
    package:com.google.android.euicc
    package:com.google.android.googlequicksearchbox.nga_resources
    package:com.google.android.carrier
    package:com.android.dynsystem
    package:com.android.angle
    package:com.android.providers.telephony.auto_generated_characteristics_rro
    package:com.google.android.bluetooth
    package:com.google.android.iwlan
    package:com.android.safetyregulatoryinfo.auto_generated_rro_product__
    package:com.android.providers.telephony.auto_generated_rro_product__
    package:com.google.android.overlay.devicelockcontroller
    package:com.google.android.connectivity.resources
    package:com.android.bips.auto_generated_rro_product__
    package:com.google.android.youtube
    package:com.android.simappdialog.auto_generated_rro_product__
    package:com.android.telephony.imsmedia
    package:com.android.externalstorage
    package:com.google.android.overlay.glanceablehubsettings2022
    package:com.android.server.telecom
)

# Convierte "package:com.xxx" -> "com.xxx"
strip_prefix() { echo "${1#package:}"; }

# Comprueba si un paquete está en la whitelist
is_kept() {
    local pkg="$1"
    local pkg_stripped
    pkg_stripped=$(strip_prefix "$pkg")
    for k in "${KEEP[@]}"; do
        if [[ "$k" == "$pkg_stripped" ]]; then
            return 0
        fi
    done
    return 1
}

echo "=== ADB Minimal Packages (Wallet, System UI, BT, WiFi, NFC, Play Store, Launcher) ==="
echo "Se deshabilitarán todos los paquetes que NO estén en la whitelist."
echo "Método: pm disable-user --user 0 (reversible)."
echo ""
read -p "¿Continuar? (s/N): " -r
[[ "${REPLY,,}" == "s" || "${REPLY,,}" == "si" ]] || exit 0

DISABLED=0
FAILED=0
SKIPPED=0

for raw in "${ALL_PACKAGES[@]}"; do
    pkg=$(strip_prefix "$raw")
    if is_kept "$raw"; then
        echo "[MANTENER] $pkg"
        ((SKIPPED++)) || true
        continue
    fi
    if adb shell pm disable-user --user 0 "$pkg" 2>/dev/null; then
        echo "[DESHABILITADO] $pkg"
        ((DISABLED++)) || true
    else
        echo "[FALLO/NO INSTALADO] $pkg"
        ((FAILED++)) || true
    fi
done

echo ""
echo "=== Resumen ==="
echo "Mantenidos: $SKIPPED"
echo "Deshabilitados: $DISABLED"
echo "Fallos / no instalados: $FAILED"
echo ""
echo "Reinicia el dispositivo para que los cambios se apliquen por completo."
echo "Para revertir un paquete: adb shell pm enable <package>"
