#!/bin/bash

echo "🔧 Iniciando instalación del proyecto P2PFS..."

# 1. Verificar si Go está instalado
if ! command -v go &> /dev/null
then
    echo "❌ Go no está instalado. Por favor instala Go (https://go.dev/dl/)."
    exit 1
fi

# 2. Descargar dependencias del proyecto
echo "📦 Descargando dependencias del proyecto..."
go mod tidy

# 3. Mostrar la IP local
echo "🌐 Tu IP local es:"
ip addr show | grep 'inet ' | grep -v 127.0.0.1 | awk '{print $2}' | cut -d/ -f1

# 4. Verificar si existe el archivo de configuración
if [ ! -f config/peers.json ]; then
    echo "⚠️ No se encontró el archivo 'config/peers.json'. Se debe crear uno."
else
    echo "✅ Archivo 'config/peers.json' encontrado. Recuerda editarlo con los datos de las máquinas."
fi

# 5. Mostrar instrucciones de ejecución
echo "🚀 Instalación completada."
echo "ℹ️ Para ejecutar el sistema, usa:"
echo "    go run ./cmd"
echo "📁 Asegúrate de estar ubicado en la raíz del proyecto."

# 6. Aplicar permisos de ejecución automáticamente
chmod +x instalar.sh
