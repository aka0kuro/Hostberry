#!/usr/bin/env bash
# Recuperación de bootloop: rehabilita paquetes críticos. Ejecutar cuando ADB responda.
set -uo pipefail
command -v adb &>/dev/null || { echo "Instala adb."; exit 1; }
echo "Esperando dispositivo..."
adb wait-for-device
sleep 2
CRITICAL=(
    com.android.systemui
    com.android.settings
    com.android.phone
    com.google.android.permissioncontroller
    com.google.android.overlay.permissioncontroller
    com.android.providers.settings
    com.google.android.gms
    com.google.android.gsf
    com.google.android.onetimeinitializer
    com.google.android.setupwizard
    com.google.android.apps.nexuslauncher
    com.android.keychain
    com.android.credentialmanager
    com.android.server.telecom
    com.android.intentresolver
    com.android.providers.telephony
    com.android.providers.contacts
    com.android.providers.media
    com.google.android.packageinstaller
    com.google.android.ext.services
    com.google.android.ext.shared
    com.android.vending
    com.android.inputdevices
    com.android.wallpaperbackup
    com.android.wallpaper.livepicker
    com.android.dreams.basic
    com.android.devicelockcontroller
    com.android.managedprovisioning
    com.android.multiuser
    com.android.localtransport
    com.android.sharedstoragebackup
    com.android.externalstorage
    com.android.backupconfirm
    com.android.emergency
    com.android.vpndialogs
    com.android.mtp
    com.android.shell
    com.android.proxyhandler
    com.android.certinstaller
    com.android.htmlviewer
    com.android.bips
    com.android.printspooler
    com.android.providers.downloads
    com.android.providers.blockednumber
    com.android.providers.userdictionary
    com.android.providers.partnerbookmarks
    com.android.bookmarkprovider
    com.android.providers.downloads.ui
    com.google.android.inputmethod.latin
    com.google.android.contacts
    com.android.mms.service
    com.android.carrierdefaultapp
    com.android.cellbroadcastreceiver
    com.android.stk
    com.android.simappdialog
    com.android.telephony.imsmedia
    com.android.ons
    com.android.qns
    com.google.android.documentsui
    com.google.android.nfc
    com.android.nfc
    com.google.android.pixelnfc
    com.google.android.bluetooth
    com.android.bluetoothmidiservice
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
    com.android.systemui.plugin.globalactions.wallet
    com.google.android.tag
    com.google.android.euicc
    com.google.euiccpixel
    com.android.se
)
for pkg in "${CRITICAL[@]}"; do
    adb shell pm enable --user 0 "$pkg" 2>/dev/null && echo "[OK] $pkg" || true
done
echo "Reinicia el dispositivo (adb reboot)."
