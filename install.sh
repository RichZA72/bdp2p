#!/bin/bash

echo "🔧 Instalador para el sistema P2P - BDD"
echo "----------------------------------------"

# Paso 1: Verificar dependencias
echo "📦 Verificando dependencias..."

if ! command -v go &> /dev/null; then
    echo "⚠️  Go no está instalado. Instalando..."
    sudo apt update
    sudo apt install -y golang
else
    echo "✅ Go ya está instalado."
fi

# Paso 2: Instalar dependencias de Go (Fyne y otras)
echo "🧱 Instalando dependencias Go (Fyne, etc)..."
go mod tidy
go install fyne.io/fyne/v2/cmd/fyne@latest

# Paso 3: Compilar proyecto
echo "🛠️ Compilando aplicación..."
go build -o p2pfs-app ./cmd

if [ -f "p2pfs-app" ]; then
    echo "✅ Compilación exitosa."
else
    echo "❌ Error al compilar."
    exit 1
fi

# Paso 4: Mensaje final
echo "🎉 Instalación completa. Ahora edita el archivo config/peers.json con:"
echo " - La IP local y puerto de esta máquina."
echo " - La lista de peers en red (con su IP, puerto, ID y Active=true para este nodo)."
echo ""
echo "Ejemplo de sección del peers.json para esta máquina:"
cat <<EOF

[
  {
    "ID": 1,
    "IP": "192.168.0.6",
    "Port": "8000",
    "Active": true
  },
  {
    "ID": 2,
    "IP": "192.168.0.7",
    "Port": "8000",
    "Active": false
  }
]
EOF

echo ""
echo "✅ Ejecuta la aplicación con: ./p2pfs-app"
