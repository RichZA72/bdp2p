# Sistema P2P Básico

Este es un sistema P2P tolerante a fallas desarrollado en Go para redes locales.

## Instrucciones

1. Instala Go.
2. Modifica `config/peers.json` con las IPs de tus nodos.
3. Ejecuta con:
   ```bash
   go run ./cmd
   ```

Cada máquina debe tener una copia del proyecto y su propia IP en `peers.json`.
