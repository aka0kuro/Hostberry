#!/bin/bash

# HostBerry - Script de Instalación para Linux
# Compatible con Debian, Ubuntu, Raspberry Pi OS

set -e  # Salir si hay algún error

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Estilos
BOLD='\033[1m'
DIM='\033[2m'

# Variables de configuración
INSTALL_DIR="/opt/hostberry"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_NAME="hostberry"
USER_NAME="hostberry"
GROUP_NAME="hostberry"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
CONFIG_FILE="${INSTALL_DIR}/config.yaml"
LOG_DIR="/var/log/hostberry"
DATA_DIR="${INSTALL_DIR}/data"
GITHUB_REPO="https://github.com/aka0kuro/Hostberry.git"
TEMP_CLONE_DIR="/tmp/hostberry-install"

# Modo de operación
MODE="install"  # install, update o uninstall

# Mensajes (concisos)
print_info()    { echo -e "${BLUE}[i]${NC} $1"; }
print_success() { echo -e "${GREEN}[+]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[!]${NC} $1"; }
print_error()   { echo -e "${RED}[x]${NC} $1"; }

# Logo ASCII (basado en website/static/hostberry.png)
print_logo() {
    printf "%b" "$RED"
    cat <<'EOF'
                       $x                    
                      $$$                    
            $$$$$$   $$$  .$$$$$$            
            $$$$$$$$ $$ .$$$$$$$             
             $$$$$$$$$$$$$$$$$$              
               $$$$$$$$$$$$$$.               
          XXXXXXXXXXXXXXXXXXXXXXXX           
        :XXXXXXXXXXXXXXXXXXXXXXXXXXX         
        XXXXXXXXXXX;:::::XXXXXXXXXXX.        
     .XXXXXXXXX:::::XXXX:::::XXXXXXXXX+      
    XXXXXXXXXXX:XXXX;:::XXXX:+XXXXXXXXXX     
    XXXXXXX::XXXX::::XX+:::XXXX::XXXXXXX     
    :XXXXX::XXXXXXXXXXXXXXXXXXXX::XXXXXX     
      XXXX:XX;:XXXXX:XX::XXXX::XX::XXX:      
      XXX+:XXX:::XXXXXXXXXX::::XX::XXX       
      XXXXXXXXXXXXXXX$XXXXXXXXXXXXXXXXx      
      XXXXXXXXX::::::::::::::XXXXXXXXX       
       XXXXXXX::X$:$X:::::::::XXXXXXX        
          XXXX::::::::::::::::XXXX           
          XXXXXX$XXXXXXXXXXXXXXXXX           
           XXXXXXXXXXXXXXXXXXXXXX            
              xXXXXXXXXXXXXXXX               
                 .XXXXXXXXX                  
                    XXXX.                    
                            

EOF
    printf "%b\n" "$NC"
}

print_banner() {
    local label="$1"
    local accent="$BLUE"
    case "$MODE" in
        install)   accent="$GREEN" ;;
        update)    accent="$BLUE" ;;
        uninstall) accent="$RED" ;;
    esac

    echo ""
    print_logo
    printf "%b\n" "${accent}${BOLD}HostBerry${NC} ${DIM}${label}${NC}"
    echo ""
}

# Procesar argumentos
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --update) MODE="update" ;;
        --uninstall) MODE="uninstall" ;;
        *) print_error "Opción desconocida: $1"; exit 1 ;;
    esac
    shift
done

# Verificar si se ejecuta como root
check_root() {
    if [ "$EUID" -ne 0 ]; then 
        print_error "Ejecuta con sudo/root"
        exit 1
    fi
}

# Configurar hostname en /etc/hosts para evitar warnings de sudo
fix_hostname() {
    CURRENT_HOSTNAME=$(hostname)
    if [ -n "$CURRENT_HOSTNAME" ]; then
        # Verificar si el hostname ya está en /etc/hosts
        if ! grep -q "127.0.0.1.*$CURRENT_HOSTNAME" /etc/hosts 2>/dev/null; then
            print_info "Configurando hostname '$CURRENT_HOSTNAME' en /etc/hosts..."
            # Agregar hostname a la línea de 127.0.0.1
            if grep -q "^127.0.0.1" /etc/hosts; then
                # La línea existe, agregar el hostname si no está
                sed -i "s/^127.0.0.1.*/& $CURRENT_HOSTNAME/" /etc/hosts 2>/dev/null || true
            else
                # La línea no existe, crearla
                echo "127.0.0.1 localhost $CURRENT_HOSTNAME" >> /etc/hosts
            fi
            # También agregar a 127.0.1.1 si no existe
            if ! grep -q "^127.0.1.1" /etc/hosts; then
                echo "127.0.1.1 $CURRENT_HOSTNAME" >> /etc/hosts
            fi
            print_success "Hostname configurado en /etc/hosts"
        fi
    fi
}

# Detectar sistema operativo
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
        print_info "Sistema: $OS $OS_VERSION"
    else
        print_error "No se pudo detectar el sistema operativo"
        exit 1
    fi
}

# Instalar git (necesario para descargar el proyecto)
install_git() {
    if ! command -v git &> /dev/null; then
        print_info "Instalando git..."
        apt-get update -qq
        apt-get install -y git
        print_success "Git listo"
    else
        print_success "Git: $(git --version)"
    fi
}

# Instalar dependencias del sistema
install_dependencies() {
    print_info "Instalando dependencias..."
    
    # Actualizar lista de paquetes
    apt-get update -qq
    
    # Instalar dependencias básicas
    DEPS="wget curl build-essential iw isc-dhcp-client"
    
    # Instalar hostapd y herramientas relacionadas
    print_info "Instalando hostapd, wpa_supplicant y herramientas WiFi..."
    
    # Instalar paquetes individualmente para identificar fallos específicos
    local failed_packages=()
    local installed_packages=()
    
    # Lista de paquetes WiFi
    local wifi_packages=("hostapd" "dnsmasq" "iptables" "wpa_supplicant")
    
    for package in "${wifi_packages[@]}"; do
        # Verificar si ya está instalado (múltiples métodos)
        local is_installed=false
        if dpkg -l | grep -q "^ii.*${package} "; then
            is_installed=true
        elif command -v "${package}" &> /dev/null; then
            is_installed=true
        elif [ "${package}" = "wpa_supplicant" ] && (command -v wpa_supplicant &> /dev/null || [ -f "/usr/sbin/wpa_supplicant" ] || [ -f "/sbin/wpa_supplicant" ]); then
            is_installed=true
        elif [ "${package}" = "hostapd" ] && (command -v hostapd &> /dev/null || [ -f "/usr/sbin/hostapd" ] || [ -f "/sbin/hostapd" ]); then
            is_installed=true
        elif [ "${package}" = "dnsmasq" ] && (command -v dnsmasq &> /dev/null || [ -f "/usr/sbin/dnsmasq" ] || [ -f "/sbin/dnsmasq" ]); then
            is_installed=true
        fi
        
        if [ "$is_installed" = true ]; then
            print_info "  ✓ ${package} ya está instalado"
            installed_packages+=("${package}")
        else
            print_info "  Instalando ${package}..."
            
            # Intentar instalar con salida visible para diagnóstico
            local install_output
            local install_exit_code
            install_output=$(apt-get install -y "${package}" 2>&1)
            install_exit_code=$?
            
            if [ $install_exit_code -eq 0 ]; then
                # Verificar que realmente se instaló
                local verify_installed=false
                if dpkg -l | grep -q "^ii.*${package} "; then
                    verify_installed=true
                elif command -v "${package}" &> /dev/null; then
                    verify_installed=true
                elif [ "${package}" = "wpa_supplicant" ] && (command -v wpa_supplicant &> /dev/null || [ -f "/usr/sbin/wpa_supplicant" ] || [ -f "/sbin/wpa_supplicant" ]); then
                    verify_installed=true
                elif [ "${package}" = "hostapd" ] && (command -v hostapd &> /dev/null || [ -f "/usr/sbin/hostapd" ] || [ -f "/sbin/hostapd" ]); then
                    verify_installed=true
                elif [ "${package}" = "dnsmasq" ] && (command -v dnsmasq &> /dev/null || [ -f "/usr/sbin/dnsmasq" ] || [ -f "/sbin/dnsmasq" ]); then
                    verify_installed=true
                fi
                
                if [ "$verify_installed" = true ]; then
                    print_success "  ✓ ${package} instalado correctamente"
                    installed_packages+=("${package}")
                else
                    print_warning "  ✗ ${package} no se instaló correctamente"
                    failed_packages+=("${package}")
                fi
            else
                # Mostrar información del error
                print_warning "  ✗ No se pudo instalar ${package}"
                
                # Verificar si el paquete está disponible en los repositorios
                if ! apt-cache search "${package}" 2>/dev/null | grep -q "^${package} "; then
                    print_warning "    El paquete ${package} no está disponible en los repositorios configurados"
                    print_info "    Intenta ejecutar: sudo apt-get update && sudo apt-get install -y ${package}"
                    
                    # Intentar actualizar repositorios y reinstalar
                    print_info "    Intentando actualizar repositorios y reinstalar..."
                    if apt-get update -qq && apt-get install -y "${package}" > /dev/null 2>&1; then
                        if dpkg -l | grep -q "^ii.*${package} " || command -v "${package}" &> /dev/null; then
                            print_success "  ✓ ${package} instalado correctamente después de actualizar repositorios"
                            installed_packages+=("${package}")
                            continue
                        fi
                    fi
                else
                    # Mostrar el error específico
                    echo "$install_output" | tail -5 | while read line; do
                        if [ -n "$line" ]; then
                            print_warning "    $line"
                        fi
                    done
                    
                    # Intentar con --fix-broken si hay problemas de dependencias
                    if echo "$install_output" | grep -q "broken\|dependenc"; then
                        print_info "    Intentando reparar dependencias rotas..."
                        if apt-get install -f -y > /dev/null 2>&1; then
                            print_info "    Reintentando instalación de ${package}..."
                            if apt-get install -y "${package}" > /dev/null 2>&1; then
                                if dpkg -l | grep -q "^ii.*${package} " || command -v "${package}" &> /dev/null; then
                                    print_success "  ✓ ${package} instalado correctamente después de reparar dependencias"
                                    installed_packages+=("${package}")
                                    continue
                                fi
                            fi
                        fi
                    fi
                fi
                
                failed_packages+=("${package}")
            fi
        fi
    done
    
    # Verificar instalación final
    local missing_critical=()
    for package in "${wifi_packages[@]}"; do
        if ! command -v "${package}" &> /dev/null && ! dpkg -l | grep -q "^ii.*${package} "; then
            missing_critical+=("${package}")
        fi
    done
    
    if [ ${#failed_packages[@]} -gt 0 ]; then
        print_warning "Los siguientes paquetes no se pudieron instalar: ${failed_packages[*]}"
        if [ ${#missing_critical[@]} -gt 0 ]; then
            print_warning "Paquetes críticos faltantes: ${missing_critical[*]}"
            print_info ""
            print_info "Para instalar manualmente, ejecuta:"
            print_info "  sudo apt-get update"
            print_info "  sudo apt-get install -y ${missing_critical[*]}"
            print_info ""
            print_info "Si algunos paquetes siguen fallando, verifica:"
            print_info "  1. Que los repositorios estén actualizados: sudo apt-get update"
            print_info "  2. Que no haya conflictos de paquetes: sudo apt-get check"
            print_info "  3. Que haya espacio en disco: df -h"
            print_info ""
            print_warning "La instalación continuará, pero algunas funciones WiFi pueden no estar disponibles."
        fi
    else
        print_success "Todos los paquetes WiFi instalados correctamente"
    fi
    
    # Verificar si Go está instalado
    if ! command -v go &> /dev/null; then
        print_info "Go no está instalado, instalando..."
        
        # Detectar arquitectura
        ARCH=$(uname -m)
        case $ARCH in
            x86_64)
                GO_ARCH="amd64"
                ;;
            armv7l|armv6l)
                GO_ARCH="armv6l"
                ;;
            aarch64)
                GO_ARCH="arm64"
                ;;
            *)
                print_warning "Arquitectura no reconocida: $ARCH, intentando instalar desde repositorio"
                apt-get install -y golang-go
                return
                ;;
        esac
        
        # Descargar e instalar Go
        GO_VERSION="1.21.5"
        GO_TAR="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
        GO_URL="https://go.dev/dl/${GO_TAR}"
        
        print_info "Descargando Go ${GO_VERSION}..."
        wget -q "${GO_URL}" -O "/tmp/${GO_TAR}"
        
        print_info "Instalando Go..."
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "/tmp/${GO_TAR}"
        rm "/tmp/${GO_TAR}"
        
        # Agregar Go al PATH
        if ! grep -q "/usr/local/go/bin" /etc/profile; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
        fi
        export PATH=$PATH:/usr/local/go/bin
        
        print_success "Go ${GO_VERSION} instalado"
    else
        print_success "Go ya está instalado: $(go version)"
        export PATH=$PATH:/usr/local/go/bin
    fi
    
    # Lua ya no se requiere - todo está en Go ahora
    
    # Verificar e instalar iw si no está disponible (ya está en DEPS, pero verificamos por si acaso)
    if ! command -v iw &> /dev/null; then
        print_info "Instalando iw (herramienta para gestión WiFi)..."
        apt-get install -y iw || print_warning "No se pudo instalar iw, puede que no esté disponible en este sistema"
    else
        print_success "iw ya está instalado"
    fi
    
    # Instalar otras dependencias
    apt-get install -y $DEPS
    
    print_success "Dependencias instaladas"
}

# Descargar proyecto de GitHub si es necesario
download_project() {
    # En modo update, verificar primero si tenemos código local con todos los archivos
    if [ "$MODE" = "update" ]; then
        # Verificar si estamos en un repositorio git válido con todos los archivos necesarios
        local has_all_files=true
        for item in "website" "locales" "main.go" "go.mod"; do
            if [ ! -e "${SCRIPT_DIR}/${item}" ]; then
                has_all_files=false
                break
            fi
        done
        
        # Si tenemos todos los archivos localmente, usar el directorio actual (preferir código local)
        if [ "$has_all_files" = true ]; then
            print_info "Modo actualización: usando código local en ${SCRIPT_DIR}"
            print_warning "⚠️  Si quieres actualizar desde GitHub, ejecuta desde un directorio vacío o sin el repo completo"
            return 0
        fi
        
        # Si no tenemos código local, descargar desde GitHub
        print_info "Modo actualización: descargando desde GitHub (no se encontró código local)..."
        
        # Limpiar directorio temporal si existe
        if [ -d "$TEMP_CLONE_DIR" ]; then
            rm -rf "$TEMP_CLONE_DIR"
        fi
        
        # Clonar repositorio
        if git clone "$GITHUB_REPO" "$TEMP_CLONE_DIR" 2>/dev/null; then
            print_success "Proyecto descargado desde GitHub"
            SCRIPT_DIR="$TEMP_CLONE_DIR"
            return 0
        else
            print_error "Error al descargar el proyecto desde GitHub"
            print_info "Verifica tu conexión a internet y que el repositorio sea accesible"
            exit 1
        fi
    fi
    
    # En modo install, verificar si estamos en un repositorio git válido con todos los archivos necesarios
    local has_all_files=true
    for item in "website" "locales" "main.go" "go.mod"; do
        if [ ! -e "${SCRIPT_DIR}/${item}" ]; then
            has_all_files=false
            break
        fi
    done
    
    # Si tenemos todos los archivos, usar el directorio actual
    if [ "$has_all_files" = true ]; then
        print_info "Usando proyecto local en ${SCRIPT_DIR}"
        return 0
    fi
    
    # Si no, descargar de GitHub
    print_info "Descargando proyecto desde GitHub..."
    
    # Limpiar directorio temporal si existe
    if [ -d "$TEMP_CLONE_DIR" ]; then
        rm -rf "$TEMP_CLONE_DIR"
    fi
    
    # Clonar repositorio
    if git clone "$GITHUB_REPO" "$TEMP_CLONE_DIR" 2>/dev/null; then
        print_success "Proyecto descargado desde GitHub"
        SCRIPT_DIR="$TEMP_CLONE_DIR"
        return 0
    else
        print_error "Error al descargar el proyecto desde GitHub"
        print_info "Verifica tu conexión a internet y que el repositorio sea accesible"
        exit 1
    fi
}

# Limpiar instalación anterior
clean_previous_installation() {
    if [ -d "$INSTALL_DIR" ]; then
        if [ "$MODE" = "update" ]; then
            # En modo actualización, preservar datos y configuración
            print_info "Modo actualización: preservando datos y configuración..."
            
            # Detener servicio si está corriendo
            if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
                print_info "Deteniendo servicio ${SERVICE_NAME}..."
                systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
                # Esperar un momento para que el servicio se detenga completamente
                sleep 2
            fi
            
            # Crear directorio temporal para guardar datos importantes
            TEMP_BACKUP_DIR="/tmp/hostberry-update-backup-$$"
            mkdir -p "$TEMP_BACKUP_DIR"
            
            # Hacer backup de la base de datos ANTES de eliminar nada
            if [ -d "$DATA_DIR" ]; then
                print_info "Guardando backup de base de datos..."
                # Copiar todo el contenido del directorio data
                if cp -r "$DATA_DIR" "$TEMP_BACKUP_DIR/data" 2>/dev/null; then
                    print_success "Backup de base de datos guardado en $TEMP_BACKUP_DIR/data"
                    # Verificar que el archivo de BD existe en el backup
                    if [ -f "$TEMP_BACKUP_DIR/data/hostberry.db" ]; then
                        DB_SIZE=$(du -h "$TEMP_BACKUP_DIR/data/hostberry.db" | cut -f1)
                        print_info "Base de datos respaldada: $DB_SIZE"
                    fi
                else
                    print_error "ERROR: No se pudo hacer backup de la base de datos"
                    print_error "Abortando actualización para proteger los datos"
                    rm -rf "$TEMP_BACKUP_DIR"
                    exit 1
                fi
            else
                print_warning "Directorio de datos no encontrado: $DATA_DIR"
            fi
            
            # Hacer backup de la configuración
            if [ -f "$CONFIG_FILE" ]; then
                print_info "Guardando backup de configuración..."
                if cp "$CONFIG_FILE" "$TEMP_BACKUP_DIR/config.yaml" 2>/dev/null; then
                    print_success "Configuración respaldada"
                else
                    print_warning "No se pudo hacer backup de la configuración"
                fi
            fi
            
            # Mover el directorio data fuera temporalmente para preservarlo
            TEMP_DATA_DIR="/tmp/hostberry-data-temp-$$"
            if [ -d "$DATA_DIR" ]; then
                print_info "Moviendo directorio de datos temporalmente para preservarlo..."
                # Verificar que el directorio data contiene la base de datos
                if [ -f "$DATA_DIR/hostberry.db" ]; then
                    DB_SIZE=$(du -h "$DATA_DIR/hostberry.db" | cut -f1)
                    print_info "Base de datos encontrada: $DB_SIZE"
                fi
                
                if mv "$DATA_DIR" "$TEMP_DATA_DIR" 2>/dev/null; then
                    print_success "Directorio de datos movido temporalmente a $TEMP_DATA_DIR"
                    # Verificar que el archivo de BD está en el directorio temporal
                    if [ -f "$TEMP_DATA_DIR/hostberry.db" ]; then
                        print_success "Base de datos preservada en directorio temporal"
                    else
                        print_warning "Advertencia: No se encontró hostberry.db en el directorio temporal"
                    fi
                else
                    print_error "ERROR: No se pudo mover el directorio de datos"
                    print_error "Abortando actualización para proteger los datos"
                    rm -rf "$TEMP_BACKUP_DIR"
                    exit 1
                fi
            else
                print_warning "Directorio de datos no existe: $DATA_DIR (primera instalación?)"
            fi
            
            # Eliminar directorio de instalación (data ya está fuera)
            print_info "Eliminando archivos antiguos (preservando datos)..."
            # Asegurarse de que no eliminamos el directorio data si aún existe
            if [ -d "$DATA_DIR" ]; then
                print_warning "Advertencia: El directorio data aún existe, moviéndolo antes de eliminar..."
                mv "$DATA_DIR" "$TEMP_DATA_DIR" 2>/dev/null || {
                    print_error "ERROR: No se pudo mover el directorio de datos antes de eliminar"
                    exit 1
                }
            fi
            rm -rf "$INSTALL_DIR"
            print_success "Archivos antiguos eliminados"
            
            # Restaurar directorio de datos
            if [ -d "$TEMP_DATA_DIR" ]; then
                print_info "Restaurando directorio de datos..."
                mkdir -p "$(dirname "$DATA_DIR")"
                if mv "$TEMP_DATA_DIR" "$DATA_DIR" 2>/dev/null; then
                    print_success "Directorio de datos restaurado"
                    # Verificar que la BD existe
                    if [ -f "$DATA_DIR/hostberry.db" ]; then
                        DB_SIZE=$(du -h "$DATA_DIR/hostberry.db" | cut -f1)
                        print_success "✅ Base de datos preservada exitosamente: $DB_SIZE"
                    else
                        print_warning "Advertencia: No se encontró hostberry.db después de restaurar"
                        # Intentar restaurar desde backup
                        if [ -d "$TEMP_BACKUP_DIR/data" ] && [ -f "$TEMP_BACKUP_DIR/data/hostberry.db" ]; then
                            print_info "Intentando restaurar desde backup..."
                            cp -r "$TEMP_BACKUP_DIR/data/"* "$DATA_DIR/" 2>/dev/null && {
                                print_success "Base de datos restaurada desde backup"
                            } || {
                                print_error "ERROR: No se pudo restaurar desde backup"
                            }
                        fi
                    fi
                else
                    print_error "ERROR: No se pudo restaurar el directorio de datos"
                    print_error "Intentando restaurar desde backup..."
                    # Intentar restaurar desde backup como fallback
                    if [ -d "$TEMP_BACKUP_DIR/data" ]; then
                        mkdir -p "$DATA_DIR"
                        if cp -r "$TEMP_BACKUP_DIR/data/"* "$DATA_DIR/" 2>/dev/null; then
                            print_success "Base de datos restaurada desde backup"
                            if [ -f "$DATA_DIR/hostberry.db" ]; then
                                DB_SIZE=$(du -h "$DATA_DIR/hostberry.db" | cut -f1)
                                print_success "Base de datos verificada: $DB_SIZE"
                            fi
                        else
                            print_error "ERROR CRÍTICO: No se pudo restaurar la base de datos"
                            print_error "El backup está en: $TEMP_BACKUP_DIR"
                            print_error "El directorio temporal está en: $TEMP_DATA_DIR"
                            exit 1
                        fi
                    else
                        print_error "ERROR CRÍTICO: No hay backup disponible"
                        print_error "El directorio temporal está en: $TEMP_DATA_DIR"
                        exit 1
                    fi
                fi
            elif [ -d "$TEMP_BACKUP_DIR/data" ]; then
                # Si no se pudo mover, restaurar desde backup
                print_info "Restaurando base de datos desde backup..."
                mkdir -p "$DATA_DIR"
                if cp -r "$TEMP_BACKUP_DIR/data/"* "$DATA_DIR/" 2>/dev/null; then
                    print_success "Base de datos restaurada desde backup"
                    if [ -f "$DATA_DIR/hostberry.db" ]; then
                        DB_SIZE=$(du -h "$DATA_DIR/hostberry.db" | cut -f1)
                        print_success "Base de datos verificada: $DB_SIZE"
                    fi
                else
                    print_error "ERROR CRÍTICO: No se pudo restaurar la base de datos"
                    print_error "El backup está en: $TEMP_BACKUP_DIR"
                    exit 1
                fi
            else
                print_warning "No se encontró directorio de datos ni backup para restaurar"
                print_info "Se creará una nueva base de datos al iniciar el servicio"
            fi
            
            # Restaurar configuración
            if [ -f "$TEMP_BACKUP_DIR/config.yaml" ]; then
                print_info "Restaurando configuración..."
                mkdir -p "$(dirname "$CONFIG_FILE")"
                cp "$TEMP_BACKUP_DIR/config.yaml" "$CONFIG_FILE" 2>/dev/null || true
            fi
            
            # Limpiar backup temporal
            rm -rf "$TEMP_BACKUP_DIR"
            
            print_success "Archivos actualizados, datos preservados"
        else
            # En modo instalación, eliminar todo
            print_info "Eliminando instalación anterior en $INSTALL_DIR..."
            
            # Detener servicio si está corriendo
            if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
                print_info "Deteniendo servicio ${SERVICE_NAME}..."
                systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
            fi
            
            # Deshabilitar servicio
            if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
                print_info "Deshabilitando servicio ${SERVICE_NAME}..."
                systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
            fi
            
            # Eliminar directorio de instalación
            rm -rf "$INSTALL_DIR"
            print_success "Instalación anterior eliminada"
        fi
    else
        print_info "No hay instalación anterior que eliminar"
    fi
}

# Crear usuario del sistema
create_user() {
    if id "$USER_NAME" &>/dev/null; then
        print_info "Usuario $USER_NAME ya existe"
        # Asegurar que el usuario esté en el grupo netdev (necesario para wpa_supplicant)
        if getent group netdev > /dev/null 2>&1; then
            if groups "$USER_NAME" | grep -q "\bnetdev\b"; then
                print_info "Usuario $USER_NAME ya está en el grupo netdev"
            else
                print_info "Agregando usuario $USER_NAME al grupo netdev..."
                usermod -a -G netdev "$USER_NAME"
                print_success "Usuario $USER_NAME agregado al grupo netdev"
            fi
        else
            print_warning "Grupo netdev no existe, creándolo..."
            groupadd -r netdev 2>/dev/null || true
            usermod -a -G netdev "$USER_NAME"
            print_success "Grupo netdev creado y usuario agregado"
        fi
    else
        print_info "Creando usuario $USER_NAME..."
        # Crear grupo netdev si no existe
        if ! getent group netdev > /dev/null 2>&1; then
            groupadd -r netdev 2>/dev/null || true
            print_info "Grupo netdev creado"
        fi
        # Crear usuario y agregarlo al grupo netdev
        useradd -r -s /bin/false -d "$INSTALL_DIR" -G netdev "$USER_NAME"
        print_success "Usuario $USER_NAME creado y agregado al grupo netdev"
    fi
}

# Copiar archivos del proyecto
install_files() {
    print_info "Instalando archivos en $INSTALL_DIR..."
    
    # Verificar que estamos en el directorio correcto con todos los archivos
    local missing_files=0
    for item in "website" "locales" "main.go" "go.mod"; do
        if [ ! -e "${SCRIPT_DIR}/${item}" ]; then
            print_warning "No se encontró '${item}' en ${SCRIPT_DIR}"
            missing_files=$((missing_files + 1))
        fi
    done

    if [ $missing_files -gt 0 ]; then
        print_error "Error: Faltan archivos del proyecto en ${SCRIPT_DIR}"
        print_info "Asegúrate de ejecutar el script desde la raíz del repositorio clonado."
        print_info "Si has descargado solo el script, necesitas descargar el proyecto completo."
        exit 1
    fi

    # Crear directorios
    mkdir -p "$INSTALL_DIR"

    mkdir -p "$LOG_DIR"
    mkdir -p "$DATA_DIR"
    # Lua ya no se usa - todo está en Go ahora
    mkdir -p "${INSTALL_DIR}/locales"
    mkdir -p "${INSTALL_DIR}/website/static"
    mkdir -p "${INSTALL_DIR}/website/templates"
    
    # Copiar archivos necesarios
    print_info "Copiando archivos del proyecto..."
    
    # Archivos Go
    cp -f "${SCRIPT_DIR}"/*.go "${INSTALL_DIR}/" 2>/dev/null || true
    cp -f "${SCRIPT_DIR}/go.mod" "${INSTALL_DIR}/" 2>/dev/null || true
    cp -f "${SCRIPT_DIR}/go.sum" "${INSTALL_DIR}/" 2>/dev/null || true
    
    # Directorios (lua ya no se usa - todo está en Go)
    if [ -d "${SCRIPT_DIR}/locales" ]; then
        cp -r "${SCRIPT_DIR}/locales/"* "${INSTALL_DIR}/locales/" 2>/dev/null || true
    fi
    
    if [ -d "${SCRIPT_DIR}/website" ]; then
        print_info "Copiando templates y archivos estáticos..."
        
        # Asegurar que los directorios destino existen
        mkdir -p "${INSTALL_DIR}/website/templates"
        mkdir -p "${INSTALL_DIR}/website/static"
        
        # Copiar templates con verificación
        if [ -d "${SCRIPT_DIR}/website/templates" ]; then
            print_info "Copiando templates desde ${SCRIPT_DIR}/website/templates..."
            if ! cp -r "${SCRIPT_DIR}/website/templates/"* "${INSTALL_DIR}/website/templates/" 2>/dev/null; then
                print_error "Error al copiar templates"
                exit 1
            fi
            TEMPLATE_COUNT=$(find "${INSTALL_DIR}/website/templates" -name "*.html" 2>/dev/null | wc -l)
            if [ "$TEMPLATE_COUNT" -gt 0 ]; then
                print_success "Templates copiados: $TEMPLATE_COUNT archivos .html"
                # Verificar que base.html y dashboard.html existen (críticos)
                if [ -f "${INSTALL_DIR}/website/templates/base.html" ]; then
                    print_success "  ✅ base.html encontrado"
                else
                    print_error "  ❌ base.html NO encontrado (CRÍTICO)"
                    exit 1
                fi
                if [ -f "${INSTALL_DIR}/website/templates/dashboard.html" ]; then
                    print_success "  ✅ dashboard.html encontrado"
                else
                    print_error "  ❌ dashboard.html NO encontrado (CRÍTICO)"
                    exit 1
                fi
                if [ -f "${INSTALL_DIR}/website/templates/login.html" ]; then
                    print_success "  ✅ login.html encontrado"
                else
                    print_error "  ❌ login.html NO encontrado (CRÍTICO)"
                    exit 1
                fi
            else
                print_error "Error: No se encontraron templates después de copiar"
                exit 1
            fi
        else
            print_error "Error: Directorio ${SCRIPT_DIR}/website/templates no existe"
            exit 1
        fi
        
        # Copiar archivos estáticos
        if [ -d "${SCRIPT_DIR}/website/static" ]; then
            print_info "Copiando archivos estáticos..."
            cp -r "${SCRIPT_DIR}/website/static/"* "${INSTALL_DIR}/website/static/" 2>/dev/null || true
            STATIC_COUNT=$(find "${INSTALL_DIR}/website/static" -type f 2>/dev/null | wc -l)
            if [ "$STATIC_COUNT" -gt 0 ]; then
                print_info "Archivos estáticos copiados: $STATIC_COUNT archivos"
            fi
        fi
    else
        print_error "Error: Directorio website no encontrado en ${SCRIPT_DIR}"
        exit 1
    fi
    
    # Configuración
    if [ ! -f "$CONFIG_FILE" ]; then
        if [ -f "${SCRIPT_DIR}/config.yaml.example" ]; then
            cp "${SCRIPT_DIR}/config.yaml.example" "$CONFIG_FILE"
            print_info "Archivo de configuración creado desde ejemplo"
        else
            # Crear config básico
            cat > "$CONFIG_FILE" <<EOF
server:
  host: "0.0.0.0"
  port: 8000
  debug: false
  read_timeout: 30
  write_timeout: 30

database:
  type: "sqlite"
  path: "${DATA_DIR}/hostberry.db"

security:
  jwt_secret: "$(openssl rand -hex 32)"
  token_expiry: 60
  bcrypt_cost: 10
  rate_limit_rps: 10
EOF
            print_info "Archivo de configuración creado con valores por defecto"
        fi
    else
        print_info "Archivo de configuración ya existe, no se sobrescribe"
    fi
    
    # Permisos
    chown -R "$USER_NAME:$GROUP_NAME" "$INSTALL_DIR"
    chown -R "$USER_NAME:$GROUP_NAME" "$LOG_DIR"
    chown -R "$USER_NAME:$GROUP_NAME" "$DATA_DIR"
    chmod 755 "$INSTALL_DIR"
    chmod 644 "$CONFIG_FILE"
    
    print_success "Archivos instalados"
}

# Compilar el proyecto
#
# Descarga de dependencias Go: reintentos y fallbacks para redes lentas o bloqueo de proxy.golang.org
try_go_mod_download() {
    local env_kv="$1"
    local attempt="$2"
    local max="$3"
    local tmp_log

    tmp_log="$(mktemp)"

    if [ -n "$env_kv" ]; then
        print_info "go mod download ($env_kv) intento ${attempt}/${max}..."
        if env $env_kv go mod download >"$tmp_log" 2>&1; then
            rm -f "$tmp_log"
            return 0
        fi
    else
        print_info "go mod download (env por defecto) intento ${attempt}/${max}..."
        if go mod download >"$tmp_log" 2>&1; then
            rm -f "$tmp_log"
            return 0
        fi
    fi

    print_warning "Falló la descarga de dependencias Go (intento ${attempt}/${max}). Últimas líneas del error:"
    tail -n 10 "$tmp_log" 2>/dev/null | while read -r line; do
        [ -n "$line" ] && print_warning "  $line"
    done
    rm -f "$tmp_log"
    return 1
}

download_go_deps() {
    local retries="${HOSTBERRY_GO_MOD_RETRIES:-5}"
    local sleep_secs="${HOSTBERRY_GO_MOD_RETRY_SLEEP:-4}"

    print_info "Go env: GOPROXY=$(go env GOPROXY 2>/dev/null || echo 'desconocido'), GOSUMDB=$(go env GOSUMDB 2>/dev/null || echo 'desconocido')"

    # 1) Intentar con el entorno actual
    for ((i=1; i<=retries; i++)); do
        if try_go_mod_download "" "$i" "$retries"; then
            export HOSTBERRY_GO_MOD_ENV=""
            return 0
        fi
        sleep "$sleep_secs"
    done

    # 2) Fallback a modo directo (sin proxy)
    print_warning "No se pudo descargar usando GOPROXY actual. Probando fallback: GOPROXY=direct"
    for ((i=1; i<=retries; i++)); do
        if try_go_mod_download "GOPROXY=direct" "$i" "$retries"; then
            export HOSTBERRY_GO_MOD_ENV="GOPROXY=direct"
            return 0
        fi
        sleep "$sleep_secs"
    done

    # 3) (Opcional) último recurso: desactivar sumdb (menos seguro)
    if [ "${HOSTBERRY_ALLOW_GOSUMDB_OFF:-0}" = "1" ]; then
        print_warning "Último recurso habilitado: GOPROXY=direct GOSUMDB=off (menos seguro)."
        for ((i=1; i<=retries; i++)); do
            if try_go_mod_download "GOPROXY=direct GOSUMDB=off" "$i" "$retries"; then
                export HOSTBERRY_GO_MOD_ENV="GOPROXY=direct GOSUMDB=off"
                return 0
            fi
            sleep "$sleep_secs"
        done
    fi

    print_error "Error al descargar dependencias de Go."
    print_info "Posibles causas: conexión lenta, DNS, bloqueo a proxy.golang.org, reloj del sistema o CA certificates."
    print_info "Puedes probar manualmente:"
    print_info "  - GOPROXY=direct go mod download"
    print_info "  - (último recurso) HOSTBERRY_ALLOW_GOSUMDB_OFF=1 GOPROXY=direct GOSUMDB=off go mod download"
    return 1
}

build_project() {
    print_info "Compilando HostBerry en ${INSTALL_DIR}..."
    
    # Verificar que estamos en el directorio correcto
    if [ ! -d "$INSTALL_DIR" ]; then
        print_error "Error: Directorio de instalación no existe: $INSTALL_DIR"
        exit 1
    fi
    
    # Cambiar al directorio de instalación
    cd "$INSTALL_DIR" || {
        print_error "Error: No se pudo cambiar al directorio $INSTALL_DIR"
        exit 1
    }
    
    print_info "Directorio de trabajo: $(pwd)"
    
    # Verificar que los templates están presentes antes de compilar
    if [ ! -d "${INSTALL_DIR}/website/templates" ]; then
        print_error "Error: Directorio de templates no encontrado: ${INSTALL_DIR}/website/templates"
        print_info "Verificando estructura del directorio..."
        ls -la "${INSTALL_DIR}/" 2>/dev/null || true
        exit 1
    fi
    
    TEMPLATE_COUNT=$(find "${INSTALL_DIR}/website/templates" -name "*.html" 2>/dev/null | wc -l)
    if [ "$TEMPLATE_COUNT" -eq 0 ]; then
        print_error "Error: No se encontraron archivos .html en ${INSTALL_DIR}/website/templates"
        print_info "Contenido del directorio:"
        ls -la "${INSTALL_DIR}/website/templates/" 2>/dev/null || true
        exit 1
    fi
    print_success "Verificado: $TEMPLATE_COUNT templates encontrados en ${INSTALL_DIR}/website/templates"
    
    # Verificar que main.go existe
    if [ ! -f "${INSTALL_DIR}/main.go" ]; then
        print_error "Error: main.go no encontrado en ${INSTALL_DIR}"
        print_info "Archivos .go encontrados:"
        ls -la "${INSTALL_DIR}"/*.go 2>/dev/null || true
        exit 1
    fi
    
    # Verificar que go.mod existe
    if [ ! -f "${INSTALL_DIR}/go.mod" ]; then
        print_error "Error: go.mod no encontrado en ${INSTALL_DIR}"
        exit 1
    fi
    
    # Asegurar que Go está en el PATH
    export PATH=$PATH:/usr/local/go/bin
    
    # Verificar que Go está disponible
    if ! command -v go &> /dev/null; then
        print_error "Error: Go no está instalado o no está en el PATH"
        exit 1
    fi
    
    print_info "Go versión: $(go version)"
    
    # Descargar dependencias
    print_info "Descargando dependencias de Go..."
    if ! download_go_deps; then
        exit 1
    fi
    
    if ! env $HOSTBERRY_GO_MOD_ENV go mod tidy; then
        print_warning "Advertencia: go mod tidy tuvo problemas, continuando..."
    fi
    
    # Verificar estructura antes de compilar
    print_info "Verificando estructura antes de compilar..."
    print_info "  - main.go: ${INSTALL_DIR}/main.go"
    if [ -f "${INSTALL_DIR}/main.go" ]; then
        print_success "  ✅ main.go encontrado"
    else
        print_error "  ❌ main.go NO encontrado"
        exit 1
    fi
    
    print_info "  - templates: ${INSTALL_DIR}/website/templates"
    if [ -d "${INSTALL_DIR}/website/templates" ]; then
        TEMPLATE_LIST=$(ls -1 "${INSTALL_DIR}/website/templates"/*.html 2>/dev/null | wc -l)
        print_success "  ✅ Directorio de templates encontrado con $TEMPLATE_LIST archivos"
        # Listar algunos templates para verificación
        print_info "  Templates encontrados:"
        ls -1 "${INSTALL_DIR}/website/templates"/*.html 2>/dev/null | head -5 | while read file; do
            print_info "    - $(basename "$file")"
        done
    else
        print_error "  ❌ Directorio de templates NO encontrado"
        exit 1
    fi
    
    # Compilar
    print_info "Compilando binario (los templates se embebarán automáticamente desde ${INSTALL_DIR}/website/templates)..."
    print_info "La directiva //go:embed buscará templates en: website/templates (relativo a main.go en ${INSTALL_DIR})"
    print_info "Directorio actual: $(pwd)"
    
    if CGO_ENABLED=1 go build -ldflags="-s -w" -o "${INSTALL_DIR}/hostberry" .; then
        if [ -f "${INSTALL_DIR}/hostberry" ]; then
            chmod +x "${INSTALL_DIR}/hostberry"
            chown "$USER_NAME:$GROUP_NAME" "${INSTALL_DIR}/hostberry"
            BINARY_SIZE=$(du -h "${INSTALL_DIR}/hostberry" | cut -f1)
            print_success "Compilación exitosa (templates embebidos en el binario)"
            print_info "Tamaño del binario: $BINARY_SIZE"
        else
            print_error "Error: El binario no se creó en ${INSTALL_DIR}/hostberry"
            exit 1
        fi
    else
        print_error "Error en la compilación"
        print_info "Revisa los errores de compilación arriba"
        exit 1
    fi
}

# Configurar firewall
configure_firewall() {
    print_info "Configurando firewall..."
    
    PORT=$(grep -E "^  port:" "$CONFIG_FILE" 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "8000")
    
    # Verificar si ufw está instalado y activo
    if command -v ufw &> /dev/null; then
        if ufw status | grep -q "Status: active"; then
            print_info "Firewall UFW activo, permitiendo puerto $PORT..."
            ufw allow "$PORT/tcp" 2>/dev/null || true
            print_success "Puerto $PORT permitido en firewall"
        else
            print_info "Firewall UFW instalado pero no activo"
        fi
    elif command -v firewall-cmd &> /dev/null; then
        # Firewalld (CentOS/RHEL)
        print_info "Configurando firewalld..."
        firewall-cmd --permanent --add-port="$PORT/tcp" 2>/dev/null || true
        firewall-cmd --reload 2>/dev/null || true
        print_success "Puerto $PORT configurado en firewalld"
    else
        print_info "No se encontró firewall configurado (ufw o firewalld)"
        print_warning "Asegúrate de permitir el puerto $PORT en tu firewall manualmente"
    fi
}

# Crear base de datos inicial
create_database() {
    print_info "Preparando base de datos..."
    
    # Asegurar que el directorio de datos existe
    mkdir -p "$DATA_DIR"
    chown -R "$USER_NAME:$GROUP_NAME" "$DATA_DIR"
    chmod 755 "$DATA_DIR"
    
    # El archivo de BD se creará automáticamente al iniciar el servicio
    # pero creamos el directorio y verificamos permisos
    DB_FILE="${DATA_DIR}/hostberry.db"
    if [ -f "$DB_FILE" ]; then
        print_info "Base de datos existente encontrada: $DB_FILE"
        chown "$USER_NAME:$GROUP_NAME" "$DB_FILE"
        chmod 644 "$DB_FILE"
        print_warning "Si la BD tiene datos antiguos, el usuario admin puede no crearse automáticamente"
    else
        print_info "Base de datos se creará automáticamente al iniciar el servicio"
        print_info "El usuario admin se creará automáticamente si la BD está vacía"
    fi
    
    print_success "Directorio de base de datos preparado: $DATA_DIR"
}

# Configurar permisos y sudoers
configure_permissions() {
    print_info "Configurando permisos y sudoers..."
    
    # Crear directorio para scripts seguros
    SAFE_DIR="/usr/local/sbin/hostberry-safe"
    mkdir -p "$SAFE_DIR"
    
    # Crear script set-timezone
    cat > "$SAFE_DIR/set-timezone" <<EOF
#!/bin/bash
TZ="\$1"
if [ -z "\$TZ" ]; then echo "Timezone required"; exit 1; fi
if [ ! -f "/usr/share/zoneinfo/\$TZ" ]; then echo "Invalid timezone"; exit 1; fi
timedatectl set-timezone "\$TZ"
EOF
    chmod 750 "$SAFE_DIR/set-timezone"
    chown root:$GROUP_NAME "$SAFE_DIR/set-timezone"
    
    # Detectar rutas de comandos WiFi
    NMCLI_PATH=""
    RFKILL_PATH=""
    IFCONFIG_PATH=""
    IW_PATH=""
    IWCONFIG_PATH=""
    
    # Buscar nmcli
    if command -v nmcli &> /dev/null; then
        NMCLI_PATH=$(command -v nmcli)
    elif [ -f "/usr/bin/nmcli" ]; then
        NMCLI_PATH="/usr/bin/nmcli"
    fi
    
    # Buscar rfkill
    if command -v rfkill &> /dev/null; then
        RFKILL_PATH=$(command -v rfkill)
    elif [ -f "/usr/sbin/rfkill" ]; then
        RFKILL_PATH="/usr/sbin/rfkill"
    fi
    
    # Buscar ifconfig
    if command -v ifconfig &> /dev/null; then
        IFCONFIG_PATH=$(command -v ifconfig)
    elif [ -f "/sbin/ifconfig" ]; then
        IFCONFIG_PATH="/sbin/ifconfig"
    elif [ -f "/usr/sbin/ifconfig" ]; then
        IFCONFIG_PATH="/usr/sbin/ifconfig"
    fi
    
    # Buscar iw (para cambiar región WiFi)
    if command -v iw &> /dev/null; then
        IW_PATH=$(command -v iw)
    elif [ -f "/usr/sbin/iw" ]; then
        IW_PATH="/usr/sbin/iw"
    elif [ -f "/sbin/iw" ]; then
        IW_PATH="/sbin/iw"
    fi
    
    # Buscar iwconfig (para gestión WiFi)
    if command -v iwconfig &> /dev/null; then
        IWCONFIG_PATH=$(command -v iwconfig)
    elif [ -f "/usr/sbin/iwconfig" ]; then
        IWCONFIG_PATH="/usr/sbin/iwconfig"
    elif [ -f "/sbin/iwconfig" ]; then
        IWCONFIG_PATH="/sbin/iwconfig"
    fi
    
    # Detectar rutas de comandos de sistema
    REBOOT_PATH=""
    SHUTDOWN_PATH=""
    
    # Buscar reboot
    if command -v reboot &> /dev/null; then
        REBOOT_PATH=$(command -v reboot)
    elif [ -f "/usr/sbin/reboot" ]; then
        REBOOT_PATH="/usr/sbin/reboot"
    elif [ -f "/sbin/reboot" ]; then
        REBOOT_PATH="/sbin/reboot"
    fi
    
    # Buscar shutdown (ya detectado arriba, pero asegurarse)
    if command -v shutdown &> /dev/null; then
        SHUTDOWN_PATH=$(command -v shutdown)
    elif [ -f "/usr/sbin/shutdown" ]; then
        SHUTDOWN_PATH="/usr/sbin/shutdown"
    elif [ -f "/sbin/shutdown" ]; then
        SHUTDOWN_PATH="/sbin/shutdown"
    fi
    
    # Configurar sudoers con configuración para evitar logs en sistemas read-only
    cat > "/etc/sudoers.d/hostberry" <<EOF
# Permisos para HostBerry
# Deshabilitar logging de sudo para evitar errores en sistemas read-only
Defaults!ALL !logfile
Defaults!ALL !syslog
$USER_NAME ALL=(ALL) NOPASSWD: $SAFE_DIR/set-timezone
EOF
    
    # Agregar permisos para shutdown si está disponible
    if [ -n "$SHUTDOWN_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SHUTDOWN_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para shutdown: $SHUTDOWN_PATH"
    fi
    
    # Agregar permisos para reboot si está disponible
    if [ -n "$REBOOT_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $REBOOT_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para reboot: $REBOOT_PATH"
    fi
    
    # También agregar permisos para systemctl (más moderno y confiable)
    if command -v systemctl &> /dev/null; then
        SYSTEMCTL_PATH=$(command -v systemctl)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH reboot" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH poweroff" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH shutdown" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para systemctl: $SYSTEMCTL_PATH"
    fi
    
    # Agregar permisos WiFi si los comandos están disponibles
    if [ -n "$NMCLI_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $NMCLI_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para nmcli: $NMCLI_PATH"
    fi
    
    if [ -n "$RFKILL_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $RFKILL_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para rfkill: $RFKILL_PATH"
    fi
    
    if [ -n "$IFCONFIG_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $IFCONFIG_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para ifconfig: $IFCONFIG_PATH"
    fi
    
    if [ -n "$IW_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $IW_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para iw: $IW_PATH"
    fi
    
    if [ -n "$IWCONFIG_PATH" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $IWCONFIG_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para iwconfig: $IWCONFIG_PATH"
    fi
    
    # Agregar permisos para hostapd y systemctl hostapd
    if command -v hostapd &> /dev/null; then
        HOSTAPD_PATH=$(command -v hostapd)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $HOSTAPD_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para hostapd: $HOSTAPD_PATH"
    fi
    
    if command -v hostapd_cli &> /dev/null; then
        HOSTAPD_CLI_PATH=$(command -v hostapd_cli)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $HOSTAPD_CLI_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para hostapd_cli: $HOSTAPD_CLI_PATH"
    fi
    
    # Agregar permisos para wpa_supplicant y wpa_cli (para modo STA)
    if command -v wpa_supplicant &> /dev/null; then
        WPA_SUPPLICANT_PATH=$(command -v wpa_supplicant)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $WPA_SUPPLICANT_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para wpa_supplicant: $WPA_SUPPLICANT_PATH"
    fi
    
    # Agregar permisos para rutas estándar de wpa_supplicant (por si no está en PATH)
    for wpa_path in "/usr/sbin/wpa_supplicant" "/sbin/wpa_supplicant" "/usr/bin/wpa_supplicant" "/bin/wpa_supplicant"; do
        if [ -f "$wpa_path" ]; then
            echo "$USER_NAME ALL=(ALL) NOPASSWD: $wpa_path" >> "/etc/sudoers.d/hostberry"
            print_info "Permisos agregados para wpa_supplicant: $wpa_path"
        fi
    done
    
    if command -v wpa_cli &> /dev/null; then
        WPA_CLI_PATH=$(command -v wpa_cli)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $WPA_CLI_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para wpa_cli: $WPA_CLI_PATH"
    elif [ -f "/usr/sbin/wpa_cli" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/sbin/wpa_cli" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para wpa_cli: /usr/sbin/wpa_cli"
    elif [ -f "/sbin/wpa_cli" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /sbin/wpa_cli" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para wpa_cli: /sbin/wpa_cli"
    fi
    
    # Agregar permisos para systemctl con wpa_supplicant
    if command -v systemctl &> /dev/null; then
        SYSTEMCTL_PATH=$(command -v systemctl)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH start wpa_supplicant" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH stop wpa_supplicant" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH restart wpa_supplicant" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH status wpa_supplicant" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH stop NetworkManager" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para systemctl wpa_supplicant"
    fi
    
    # Agregar permisos para systemctl con hostapd y dnsmasq
    if command -v systemctl &> /dev/null; then
        SYSTEMCTL_PATH=$(command -v systemctl)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH start hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH stop hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH restart hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH status hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH enable hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH disable hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH unmask hostapd" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH start dnsmasq" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH stop dnsmasq" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH restart dnsmasq" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH enable dnsmasq" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH disable dnsmasq" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH unmask dnsmasq" >> "/etc/sudoers.d/hostberry"
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSTEMCTL_PATH daemon-reload" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para systemctl hostapd y dnsmasq"
    fi
    
    # Agregar permisos para hostnamectl y hostname (cambio de hostname)
    if command -v hostnamectl &> /dev/null; then
        HOSTNAMECTL_PATH=$(command -v hostnamectl)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $HOSTNAMECTL_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para hostnamectl: $HOSTNAMECTL_PATH"
    elif [ -f "/usr/bin/hostnamectl" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/hostnamectl" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para hostnamectl: /usr/bin/hostnamectl"
    fi
    
    if command -v hostname &> /dev/null; then
        HOSTNAME_PATH=$(command -v hostname)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $HOSTNAME_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para hostname: $HOSTNAME_PATH"
    elif [ -f "/bin/hostname" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/hostname" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para hostname: /bin/hostname"
    fi
    
    # Agregar permisos para ip (configuración de interfaces de red)
    if command -v ip &> /dev/null; then
        IP_PATH=$(command -v ip)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $IP_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para ip: $IP_PATH"
    elif [ -f "/usr/sbin/ip" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/sbin/ip" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para ip: /usr/sbin/ip"
    elif [ -f "/sbin/ip" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /sbin/ip" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para ip: /sbin/ip"
    fi
    
    # Agregar permisos para pkill (para detener procesos wpa_supplicant)
    if command -v pkill &> /dev/null; then
        PKILL_PATH=$(command -v pkill)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $PKILL_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para pkill: $PKILL_PATH"
    elif [ -f "/usr/bin/pkill" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/pkill" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para pkill: /usr/bin/pkill"
    fi
    
    # Agregar permisos para pgrep (para verificar procesos)
    if command -v pgrep &> /dev/null; then
        PGREP_PATH=$(command -v pgrep)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $PGREP_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para pgrep: $PGREP_PATH"
    elif [ -f "/usr/bin/pgrep" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/pgrep" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para pgrep: /usr/bin/pgrep"
    fi
    
    # Agregar permisos para dhclient y udhcpc (para obtener IP)
    if command -v dhclient &> /dev/null; then
        DHCPCLIENT_PATH=$(command -v dhclient)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $DHCPCLIENT_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para dhclient: $DHCPCLIENT_PATH"
    elif [ -f "/usr/sbin/dhclient" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/sbin/dhclient" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para dhclient: /usr/sbin/dhclient"
    fi
    
    if command -v udhcpc &> /dev/null; then
        UDHCPC_PATH=$(command -v udhcpc)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $UDHCPC_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para udhcpc: $UDHCPC_PATH"
    elif [ -f "/usr/sbin/udhcpc" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/sbin/udhcpc" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para udhcpc: /usr/sbin/udhcpc"
    fi
    
    # Agregar permisos para sysctl (habilitar IP forwarding)
    if command -v sysctl &> /dev/null; then
        SYSCTL_PATH=$(command -v sysctl)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SYSCTL_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para sysctl: $SYSCTL_PATH"
    elif [ -f "/usr/sbin/sysctl" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/sbin/sysctl" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para sysctl: /usr/sbin/sysctl"
    elif [ -f "/sbin/sysctl" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /sbin/sysctl" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para sysctl: /sbin/sysctl"
    fi
    
    # Agregar permisos para iptables (configuración de NAT)
    if command -v iptables &> /dev/null; then
        IPTABLES_PATH=$(command -v iptables)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $IPTABLES_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para iptables: $IPTABLES_PATH"
    elif [ -f "/usr/sbin/iptables" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/sbin/iptables" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para iptables: /usr/sbin/iptables"
    elif [ -f "/sbin/iptables" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /sbin/iptables" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para iptables: /sbin/iptables"
    fi
    
    # Agregar permisos para comandos básicos necesarios para hostapd
    # cp (para copiar archivos de configuración)
    if command -v cp &> /dev/null; then
        CP_PATH=$(command -v cp)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $CP_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para cp: $CP_PATH"
    elif [ -f "/bin/cp" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/cp" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para cp: /bin/cp"
    fi
    
    # mkdir (para crear directorios de configuración)
    if command -v mkdir &> /dev/null; then
        MKDIR_PATH=$(command -v mkdir)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $MKDIR_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para mkdir: $MKDIR_PATH"
    elif [ -f "/bin/mkdir" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/mkdir" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para mkdir: /bin/mkdir"
    fi
    
    # chmod (para establecer permisos de archivos)
    if command -v chmod &> /dev/null; then
        CHMOD_PATH=$(command -v chmod)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $CHMOD_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para chmod: $CHMOD_PATH"
    elif [ -f "/bin/chmod" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/chmod" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para chmod: /bin/chmod"
    fi
    
    # tee (para escribir archivos de configuración)
    if command -v tee &> /dev/null; then
        TEE_PATH=$(command -v tee)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $TEE_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para tee: $TEE_PATH"
    elif [ -f "/usr/bin/tee" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/tee" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para tee: /usr/bin/tee"
    fi
    
    # cat (para leer archivos y pasarlos a tee)
    if command -v cat &> /dev/null; then
        CAT_PATH=$(command -v cat)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $CAT_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para cat: $CAT_PATH"
    elif [ -f "/bin/cat" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/cat" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para cat: /bin/cat"
    fi
    
    # grep (para buscar en archivos como /etc/hosts)
    if command -v grep &> /dev/null; then
        GREP_PATH=$(command -v grep)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $GREP_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para grep: $GREP_PATH"
    elif [ -f "/bin/grep" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/grep" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para grep: /bin/grep"
    elif [ -f "/usr/bin/grep" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/grep" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para grep: /usr/bin/grep"
    fi
    
    # sed (para reemplazar texto en archivos como /etc/hosts)
    if command -v sed &> /dev/null; then
        SED_PATH=$(command -v sed)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $SED_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para sed: $SED_PATH"
    elif [ -f "/bin/sed" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/sed" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para sed: /bin/sed"
    elif [ -f "/usr/bin/sed" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/sed" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para sed: /usr/bin/sed"
    fi
    
    # mount (para remontar sistemas de archivos de solo lectura como lectura-escritura)
    if command -v mount &> /dev/null; then
        MOUNT_PATH=$(command -v mount)
        echo "$USER_NAME ALL=(ALL) NOPASSWD: $MOUNT_PATH" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para mount: $MOUNT_PATH"
    elif [ -f "/bin/mount" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /bin/mount" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para mount: /bin/mount"
    elif [ -f "/usr/bin/mount" ]; then
        echo "$USER_NAME ALL=(ALL) NOPASSWD: /usr/bin/mount" >> "/etc/sudoers.d/hostberry"
        print_info "Permisos agregados para mount: /usr/bin/mount"
    fi
    
    # Crear directorio /etc/hostapd con permisos correctos
    print_info "Creando directorio /etc/hostapd..."
    if [ ! -d "/etc/hostapd" ]; then
        mkdir -p /etc/hostapd
        chmod 755 /etc/hostapd
        print_success "Directorio /etc/hostapd creado con permisos 755"
    else
        chmod 755 /etc/hostapd 2>/dev/null || true
        print_info "Directorio /etc/hostapd ya existe, permisos verificados"
    fi
    
    # Crear también el directorio para systemd override si no existe
    if [ ! -d "/etc/systemd/system/hostapd.service.d" ]; then
        mkdir -p /etc/systemd/system/hostapd.service.d
        print_info "Directorio systemd override para hostapd creado"
    fi
    
    # Validar configuración de sudoers
    if visudo -c -f "/etc/sudoers.d/hostberry" 2>/dev/null; then
        chmod 440 "/etc/sudoers.d/hostberry"
        print_success "Permisos y sudoers configurados correctamente"
    else
        print_warning "Advertencia: Error al validar configuración de sudoers"
        print_info "Revisa manualmente: visudo -c -f /etc/sudoers.d/hostberry"
        chmod 440 "/etc/sudoers.d/hostberry"
    fi

    # Crear directorio temporal persistente para la configuración de wpa_supplicant
    FALLBACK_WPA_DIR="/tmp/hostberry/wpa_supplicant"
    if [ ! -d "$FALLBACK_WPA_DIR" ]; then
        mkdir -p "$FALLBACK_WPA_DIR"
        chown root:netdev "$FALLBACK_WPA_DIR"
        chmod 775 "$FALLBACK_WPA_DIR"
        print_info "Directorio temporal persistente creado: $FALLBACK_WPA_DIR"
    else
        chown root:netdev "$FALLBACK_WPA_DIR"
        chmod 775 "$FALLBACK_WPA_DIR"
    fi
}

# Crear configuración por defecto de HostAPD
create_hostapd_default_config() {
    print_info "Creando configuración por defecto de HostAPD..."
    
    # Valores por defecto
    HOSTAPD_INTERFACE="wlan0"
    HOSTAPD_SSID="hostberry-ap"
    HOSTAPD_PASSWORD="hostberry12"
    HOSTAPD_CHANNEL="6"
    HOSTAPD_GATEWAY="192.168.4.1"
    HOSTAPD_DHCP_START="192.168.4.2"
    HOSTAPD_DHCP_END="192.168.4.254"
    HOSTAPD_LEASE_TIME="12h"
    
    # Crear archivo de configuración de hostapd si no existe
    # Modo AP+STA según método del blog de TheWalrus (Raspberry Pi 3 B+)
    # Usar interfaz virtual ap0 para el AP, permitiendo que wlan0 funcione como estación (STA)
    HOSTAPD_CONFIG="/etc/hostapd/hostapd.conf"
    if [ ! -f "$HOSTAPD_CONFIG" ]; then
        print_info "Creando archivo de configuración de HostAPD (modo AP+STA según TheWalrus - Raspberry Pi 3 B+): $HOSTAPD_CONFIG"
        
        # Validar interfaz WiFi
        if [ ! -d "/sys/class/net/${HOSTAPD_INTERFACE}" ]; then
            print_warning "Interfaz WiFi no encontrada: ${HOSTAPD_INTERFACE}. Se usará esa interfaz si existe luego."
        fi

        # Verificar si iw está disponible para gestionar interfaces virtuales
        if ! command -v iw &> /dev/null; then
            print_warning "iw no está disponible; no se puede crear ap0. Se usará la interfaz física."
            AP_INTERFACE="$HOSTAPD_INTERFACE"
        fi

        # Obtener el phy de la interfaz WiFi (si iw está disponible)
        PHY_NAME=""
        if command -v iw &> /dev/null; then
            PHY_NAME=$(iw dev "$HOSTAPD_INTERFACE" info 2>/dev/null | grep wiphy | awk '{print $2}')
            if [ -z "$PHY_NAME" ]; then
                PHY_NAME=$(cat /sys/class/net/"$HOSTAPD_INTERFACE"/phy80211/name 2>/dev/null || true)
            fi
            if [ -z "$PHY_NAME" ]; then
                PHY_NAME="phy0"
            fi
        fi
        
        # Obtener MAC address de la interfaz física para la regla udev
        MAC_ADDRESS=$(cat /sys/class/net/"$HOSTAPD_INTERFACE"/address 2>/dev/null || echo "")
        
        # Crear regla udev para crear ap0 automáticamente al arrancar (método TheWalrus - Raspberry Pi 3 B+)
        if [ -n "$MAC_ADDRESS" ] && [ -n "$PHY_NAME" ]; then
            print_info "Creando regla udev para ap0 (método TheWalrus - Raspberry Pi 3 B+)..."
            UDEV_RULE="/etc/udev/rules.d/70-persistent-net.rules"
            if [ ! -f "$UDEV_RULE" ] || ! grep -q "ap0" "$UDEV_RULE" 2>/dev/null; then
                cat >> "$UDEV_RULE" <<EOF

# Regla para crear interfaz virtual ap0 automáticamente (método TheWalrus - Raspberry Pi 3 B+)
SUBSYSTEM=="ieee80211", ACTION=="add|change", ATTR{macaddress}=="$MAC_ADDRESS", KERNEL=="$PHY_NAME", \
RUN+="/sbin/iw phy $PHY_NAME interface add ap0 type __ap", \
RUN+="/bin/ip link set ap0 address $MAC_ADDRESS"
EOF
                chmod 644 "$UDEV_RULE"
                print_success "Regla udev creada para ap0"
                # Recargar reglas udev
                udevadm control --reload-rules 2>/dev/null || true
                udevadm trigger 2>/dev/null || true
            else
                print_info "Regla udev para ap0 ya existe"
            fi
        fi
        
        # Intentar crear interfaz virtual ap0 si no existe (solo si iw está disponible)
        if command -v iw &> /dev/null; then
            if ! ip link show ap0 > /dev/null 2>&1; then
                print_info "Creando interfaz virtual ap0 para modo AP+STA..."
                
                # Asegurar que la interfaz física esté activa
                ip link set "$HOSTAPD_INTERFACE" up 2>/dev/null || true
                sleep 1
                
                # Intentar crear ap0 con múltiples métodos
                AP_CREATED=false
                
                # Método 1: Usando phy directamente
                if [ -n "$PHY_NAME" ] && iw phy "$PHY_NAME" interface add ap0 type __ap 2>/dev/null; then
                    AP_CREATED=true
                    print_success "Interfaz virtual ap0 creada usando phy $PHY_NAME"
                # Método 2: Usando la interfaz directamente
                elif iw dev "$HOSTAPD_INTERFACE" interface add ap0 type __ap 2>/dev/null; then
                    AP_CREATED=true
                    print_success "Interfaz virtual ap0 creada usando interfaz $HOSTAPD_INTERFACE"
                fi
                
                if [ "$AP_CREATED" = true ]; then
                    # Configurar MAC address de ap0 igual a wlan0
                    if [ -n "$MAC_ADDRESS" ]; then
                        ip link set ap0 address "$MAC_ADDRESS" 2>/dev/null || true
                    fi
                    # Activar la interfaz
                    ip link set ap0 up 2>/dev/null || true
                    sleep 1
                    
                    # Verificar que se creó correctamente
                    if ip link show ap0 > /dev/null 2>&1; then
                        print_success "Interfaz virtual ap0 verificada y activa"
                    else
                        print_warning "ap0 se creó pero no está disponible"
                    fi
                else
                    print_warning "No se pudo crear interfaz virtual ap0, usando interfaz física directamente"
                    print_info "Sugerencia: tu driver puede no soportar AP+STA. Verifica con: iw list | grep -A5 -i 'valid interface combinations'"
                    AP_INTERFACE="$HOSTAPD_INTERFACE"
                fi
            else
                print_success "Interfaz virtual ap0 ya existe"
                # Asegurar que esté activa
                ip link set ap0 up 2>/dev/null || true
            fi
        else
            print_warning "iw no está disponible, no se puede crear ap0"
            AP_INTERFACE="$HOSTAPD_INTERFACE"
        fi

        # Usar ap0 si existe, sino usar la interfaz física
        AP_INTERFACE="$HOSTAPD_INTERFACE"
        if ip link show ap0 > /dev/null 2>&1; then
            AP_INTERFACE="ap0"
            print_info "Usando interfaz virtual ap0 (modo AP+STA según TheWalrus - Raspberry Pi 3 B+)"
        else
            print_info "Usando interfaz física $AP_INTERFACE (modo no concurrente)"
        fi
        
        cat > "$HOSTAPD_CONFIG" <<EOF
# Configuración de HostAPD para modo AP+STA según método TheWalrus (Raspberry Pi 3 B+)
# Interfaz virtual ap0 para AP, wlan0 para STA
interface=${AP_INTERFACE}
driver=nl80211
ssid=${HOSTAPD_SSID}
hw_mode=g
channel=${HOSTAPD_CHANNEL}
wpa=2
wpa_passphrase=${HOSTAPD_PASSWORD}
wpa_key_mgmt=WPA-PSK
wpa_pairwise=TKIP
rsn_pairwise=CCMP
# Asegurar que wlan0 esté en modo managed (no AP)
# Esto se hace automáticamente cuando wpa_supplicant se ejecuta en wlan0
EOF
        chmod 644 "$HOSTAPD_CONFIG"
        print_success "Archivo de configuración de HostAPD creado con valores por defecto"
        print_info "  - Interfaz AP: $AP_INTERFACE"
        print_info "  - Interfaz STA: $HOSTAPD_INTERFACE (para wpa_supplicant)"
        print_info "  - SSID: $HOSTAPD_SSID"
        print_info "  - Contraseña: $HOSTAPD_PASSWORD"
        print_info "  - Gateway: $HOSTAPD_GATEWAY"
    else
        print_info "Archivo de configuración de HostAPD ya existe, no se sobrescribe"
    fi
    
    # Crear archivo de configuración de dnsmasq si no existe o hacer backup
    DNSMASQ_CONFIG="/etc/dnsmasq.conf"
    if [ -f "$DNSMASQ_CONFIG" ]; then
        # Hacer backup si no existe
        if [ ! -f "${DNSMASQ_CONFIG}.backup" ]; then
            cp "$DNSMASQ_CONFIG" "${DNSMASQ_CONFIG}.backup"
            print_info "Backup de configuración de dnsmasq creado"
        fi
        # Verificar si ya tiene configuración de hostapd
        # En modo AP+STA, usar ap0 si existe, sino usar la interfaz física
        DNSMASQ_INTERFACE="ap0"
        if ! ip link show ap0 > /dev/null 2>&1; then
            DNSMASQ_INTERFACE="$HOSTAPD_INTERFACE"
        fi
        
        if grep -q "interface=${DNSMASQ_INTERFACE}" "$DNSMASQ_CONFIG" 2>/dev/null || \
           grep -q "interface=${HOSTAPD_INTERFACE}" "$DNSMASQ_CONFIG" 2>/dev/null || \
           grep -q "interface=ap0" "$DNSMASQ_CONFIG" 2>/dev/null; then
            print_info "Configuración de dnsmasq para HostAPD ya existe"
        else
            print_info "Agregando configuración de dnsmasq para HostAPD (modo AP+STA según TheWalrus - Raspberry Pi 3 B+)..."
            cat >> "$DNSMASQ_CONFIG" <<EOF

# Configuración para HostAPD (agregada por HostBerry) - Modo AP+STA según TheWalrus (Raspberry Pi 3 B+)
# Solo servir DHCP en ap0, no en wlan0 (que es STA)
interface=${DNSMASQ_INTERFACE}
no-dhcp-interface=${HOSTAPD_INTERFACE}
bind-interfaces
dhcp-range=${HOSTAPD_DHCP_START},${HOSTAPD_DHCP_END},255.255.255.0,${HOSTAPD_LEASE_TIME}
dhcp-option=3,${HOSTAPD_GATEWAY}
dhcp-option=6,${HOSTAPD_GATEWAY}
server=8.8.8.8
server=8.8.4.4
domain-needed
bogus-priv
EOF
            print_success "Configuración de dnsmasq actualizada"
        fi
    else
        # Crear archivo de configuración de dnsmasq desde cero
        # En modo AP+STA, usar ap0 si existe
        DNSMASQ_INTERFACE="ap0"
        if ! ip link show ap0 > /dev/null 2>&1; then
            DNSMASQ_INTERFACE="$HOSTAPD_INTERFACE"
        fi
        
        print_info "Creando archivo de configuración de dnsmasq (modo AP+STA según TheWalrus - Raspberry Pi 3 B+)..."
        cat > "$DNSMASQ_CONFIG" <<EOF
# Configuración de dnsmasq para HostAPD (creada por HostBerry) - Modo AP+STA según TheWalrus (Raspberry Pi 3 B+)
# Solo servir DHCP en ap0, no en wlan0 (que es STA)
interface=${DNSMASQ_INTERFACE}
no-dhcp-interface=${HOSTAPD_INTERFACE}
bind-interfaces
dhcp-range=${HOSTAPD_DHCP_START},${HOSTAPD_DHCP_END},255.255.255.0,${HOSTAPD_LEASE_TIME}
dhcp-option=3,${HOSTAPD_GATEWAY}
dhcp-option=6,${HOSTAPD_GATEWAY}
server=8.8.8.8
server=8.8.4.4
domain-needed
bogus-priv
EOF
        chmod 644 "$DNSMASQ_CONFIG"
        print_success "Archivo de configuración de dnsmasq creado"
    fi
    
# Configurar wpa_supplicant para modo STA
print_info "Configurando wpa_supplicant para modo estación (STA)..."

# Crear directorio de configuración de wpa_supplicant
print_info "Creando directorio de configuración de wpa_supplicant..."
mkdir -p /etc/wpa_supplicant
chown root:netdev /etc/wpa_supplicant 2>/dev/null || chown root:root /etc/wpa_supplicant
chmod 755 /etc/wpa_supplicant
print_success "Directorio /etc/wpa_supplicant configurado"

# Crear directorio de socket de control de wpa_supplicant
print_info "Creando directorio de socket de control de wpa_supplicant..."
mkdir -p /var/run/wpa_supplicant
chown root:netdev /var/run/wpa_supplicant 2>/dev/null || chown root:root /var/run/wpa_supplicant
chmod 775 /var/run/wpa_supplicant
print_success "Directorio /var/run/wpa_supplicant configurado con permisos 775"

# También crear /run/wpa_supplicant (algunos sistemas usan este)
mkdir -p /run/wpa_supplicant
chown root:netdev /run/wpa_supplicant 2>/dev/null || chown root:root /run/wpa_supplicant
chmod 775 /run/wpa_supplicant
print_success "Directorio /run/wpa_supplicant configurado con permisos 775"

# Crear archivo de configuración base de wpa_supplicant
WPA_CONFIG="/etc/wpa_supplicant/wpa_supplicant-wlan0.conf"
if [ ! -f "$WPA_CONFIG" ]; then
    print_info "Creando archivo de configuración de wpa_supplicant: $WPA_CONFIG"
    cat > "$WPA_CONFIG" <<EOF
ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
ctrl_interface_group=netdev
update_config=1
country=US

# Redes guardadas se agregarán aquí automáticamente
EOF
    chmod 600 "$WPA_CONFIG"
    chown root:root "$WPA_CONFIG"
    print_success "Archivo de configuración de wpa_supplicant creado"
else
    print_info "Archivo de configuración de wpa_supplicant ya existe"
    # Verificar que tenga el grupo netdev en ctrl_interface
    if ! grep -q "GROUP=netdev" "$WPA_CONFIG" 2>/dev/null; then
        print_info "Actualizando archivo de configuración para incluir GROUP=netdev..."
        sed -i 's|ctrl_interface=DIR=/var/run/wpa_supplicant|ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev|g' "$WPA_CONFIG" 2>/dev/null || true
    fi
fi
    
    # Crear servicio systemd para crear ap0 al arrancar (si se necesita)
    if command -v iw &> /dev/null && [ -n "$PHY_NAME" ] && [ -n "$MAC_ADDRESS" ]; then
        AP0_SERVICE="/etc/systemd/system/create-ap0.service"
        if [ ! -f "$AP0_SERVICE" ]; then
            print_info "Creando servicio systemd para crear ap0 al arrancar..."
            cat > "$AP0_SERVICE" <<EOF
[Unit]
Description=Create virtual WiFi interface ap0 for AP+STA mode
After=network-pre.target
Before=network.target hostapd.service
Wants=network-pre.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/bash -c 'if ! ip link show ap0 > /dev/null 2>&1; then /sbin/iw phy ${PHY_NAME} interface add ap0 type __ap && /bin/ip link set ap0 address ${MAC_ADDRESS} && /bin/ip link set ap0 up; fi'
ExecStop=/bin/bash -c 'if ip link show ap0 > /dev/null 2>&1; then /bin/ip link set ap0 down && /sbin/iw dev ap0 del; fi'

[Install]
WantedBy=multi-user.target
EOF
            chmod 644 "$AP0_SERVICE"
            systemctl daemon-reload 2>/dev/null || true
            systemctl enable create-ap0.service 2>/dev/null || true
            systemctl start create-ap0.service 2>/dev/null || true
            print_success "Servicio systemd para ap0 creado, habilitado e iniciado"
            
            # Esperar un momento y verificar que ap0 se creó
            sleep 2
            if ip link show ap0 > /dev/null 2>&1; then
                print_success "Interfaz ap0 creada y verificada por el servicio systemd"
            else
                print_warning "El servicio se inició pero ap0 no está disponible aún (puede necesitar reinicio)"
            fi
        else
            print_info "Servicio systemd para ap0 ya existe"
        fi
    fi
    
    # Crear archivo de override de systemd para hostapd si no existe
    OVERRIDE_DIR="/etc/systemd/system/hostapd.service.d"
    OVERRIDE_FILE="${OVERRIDE_DIR}/override.conf"
    if [ ! -f "$OVERRIDE_FILE" ]; then
        print_info "Creando archivo de override de systemd para hostapd..."
        mkdir -p "$OVERRIDE_DIR"
        cat > "$OVERRIDE_FILE" <<EOF
[Unit]
After=create-ap0.service
Requires=create-ap0.service

[Service]
ExecStart=
ExecStart=/usr/sbin/hostapd -B ${HOSTAPD_CONFIG}
EOF
        chmod 644 "$OVERRIDE_FILE"
        print_success "Archivo de override de systemd creado"
    else
        print_info "Archivo de override de systemd ya existe"
    fi
    
    # Asegurarse de que el servicio no esté masked
    print_info "Verificando estado del servicio hostapd..."
    if systemctl is-enabled hostapd 2>&1 | grep -q "masked"; then
        print_info "Desbloqueando servicio hostapd..."
        systemctl unmask hostapd 2>/dev/null || true
        print_success "Servicio hostapd desbloqueado"
    fi
    
    # Recargar systemd para aplicar cambios
    systemctl daemon-reload 2>/dev/null || true
    
    # Asegurar permisos correctos del archivo de configuración
    chmod 644 "$HOSTAPD_CONFIG" 2>/dev/null || true
    
    print_success "Configuración por defecto de HostAPD creada"
}

# Crear servicio systemd
create_systemd_service() {
    print_info "Creando servicio systemd..."
    
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=HostBerry - Sistema de Gestión de Red
After=network.target

[Service]
Type=simple
User=${USER_NAME}
Group=${GROUP_NAME}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/hostberry
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

# Seguridad
# NoNewPrivileges=true  # Deshabilitado para permitir sudo en comandos WiFi
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${INSTALL_DIR} ${LOG_DIR} ${DATA_DIR}

# Recursos
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    
    # Recargar systemd
    systemctl daemon-reload
    
    print_success "Servicio systemd creado: $SERVICE_FILE"
}

# Iniciar servicio
start_service() {
    print_info "Iniciando servicio ${SERVICE_NAME}..."
    
    systemctl enable "${SERVICE_NAME}"
    systemctl start "${SERVICE_NAME}"
    systemctl restart "${SERVICE_NAME}"
    
    # Esperar un momento y verificar
    sleep 2
    
    if systemctl is-active --quiet "${SERVICE_NAME}"; then
        print_success "Servicio iniciado correctamente"
        print_info "Estado: $(systemctl is-active ${SERVICE_NAME})"
        
        # Esperar un poco más para que se cree el usuario admin
        sleep 2
        
        # Verificar si se creó el usuario admin
        print_info "Verificando creación de usuario admin..."
        if journalctl -u "${SERVICE_NAME}" -n 20 --no-pager | grep -q "Usuario admin creado exitosamente"; then
            print_success "Usuario admin creado correctamente"
        elif journalctl -u "${SERVICE_NAME}" -n 20 --no-pager | grep -q "Error creando usuario admin"; then
            print_warning "Hubo un error al crear el usuario admin"
            print_info "Revisa los logs: sudo journalctl -u ${SERVICE_NAME} -n 50"
        else
            print_info "Revisa los logs para ver el estado del usuario admin:"
            print_info "  sudo journalctl -u ${SERVICE_NAME} -n 50 | grep -i admin"
        fi
    else
        print_warning "El servicio no se inició correctamente"
        print_info "Revisa los logs con: journalctl -u ${SERVICE_NAME} -f"
    fi
}

# Mostrar información final
show_final_info() {
    echo ""
    case "$MODE" in
        update)    print_success "Actualización completa" ;;
        uninstall) print_success "Desinstalación completa" ;;
        *)         print_success "Instalación completa" ;;
    esac

    # Para desinstalación, no hay endpoints/paths que mostrar
    if [ "$MODE" = "uninstall" ]; then
        echo ""
        return 0
    fi

    local ip port web_url
    ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
    port="$(awk '/^[[:space:]]*port:/{gsub(/"/,"",$2); print $2; exit}' "$CONFIG_FILE" 2>/dev/null)"
    port="${port:-8000}"

    if [ -n "$ip" ] && [ "$ip" != "127.0.0.1" ]; then
        web_url="http://${ip}:${port}"
    else
        web_url="http://localhost:${port}"
    fi

    print_info "Web:    ${web_url}"
    print_info "Config: ${CONFIG_FILE}"
    print_info "Logs:   journalctl -u ${SERVICE_NAME} -f"
    print_warning "Login:  admin / admin (cámbiala)"
    echo ""
}

# Limpiar directorio temporal al finalizar
cleanup_temp() {
    if [ -d "$TEMP_CLONE_DIR" ] && [ "$TEMP_CLONE_DIR" != "$SCRIPT_DIR" ]; then
        print_info "Limpiando directorio temporal..."
        rm -rf "$TEMP_CLONE_DIR"
    fi
}

# Función principal
main() {
    local mode_label="INSTALACIÓN"
    if [ "$MODE" = "update" ]; then
        mode_label="ACTUALIZACIÓN"
    elif [ "$MODE" = "uninstall" ]; then
        mode_label="DESINSTALACIÓN"
    fi

    print_banner "$mode_label"
    
    check_root
    
    # Desinstalación: solo limpiar y salir (sin tocar /etc/hosts ni instalar deps)
    if [ "$MODE" = "uninstall" ]; then
        clean_previous_installation
        cleanup_temp
        show_final_info
        return 0
    fi

    fix_hostname
    detect_os

    install_git
    download_project
    clean_previous_installation
    install_dependencies
    create_user
    install_files
    build_project
    create_database
    configure_permissions
    create_hostapd_default_config
    configure_firewall
    create_systemd_service
    start_service
    cleanup_temp
    show_final_info
}

# Ejecutar función principal
main
