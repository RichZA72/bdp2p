#!/bin/bash

echo "==============================="
echo " Instalador del sistema BDP2P "
echo "==============================="

# Verificar permisos de superusuario
if [ "$EUID" -ne 0 ]; then
    echo "❌ Este script requiere permisos de superusuario. Ejecuta con: sudo ./instalar.sh"
    exit 1
fi

# Paso 1: Actualizar el sistema y herramientas esenciales
echo "🔄 Actualizando sistema y herramientas esenciales..."
apt update -y && apt install -y wget tar git curl build-essential

# Paso 2: Instalar Go (forzado)
echo "⚙️ Forzando instalación de Go..."

GO_VERSION="1.22.0"
ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    GO_ARCH="amd64"
elif [ "$ARCH" = "aarch64" ]; then
    GO_ARCH="arm64"
else
    echo "❌ Arquitectura no compatible: $ARCH"
    exit 1
fi

# Eliminar instalación anterior si existe
rm -rf /usr/local/go

# Descargar e instalar Go
wget https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz -O /tmp/go.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz

# Configurar PATH
if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
fi
export PATH=$PATH:/usr/local/go/bin

# Verificar instalación
echo "✅ Go instalado:"
go version || { echo "❌ Error al instalar Go"; exit 1; }

# Paso 3: Mostrar IP de la máquina
echo "------------------------------"
echo "📡 Dirección IP local:"
ip addr show | grep "inet " | grep -v "127.0.0.1" | awk '{print $2}' | cut -d/ -f1
echo "------------------------------"

# Paso 4: Instrucciones adicionales
echo "📁 Edita el archivo config/peers.json para ajustar tu IP y ID."

echo ""
echo "🚀 Para ejecutar el programa desde la raíz del proyecto, usa:"
echo "  go run ./cmd"
echo ""

echo "✅ Instalación finalizada. Reinicia la terminal si es necesario."
