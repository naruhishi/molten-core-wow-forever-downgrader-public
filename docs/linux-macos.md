# Molten Core en Linux y macOS

## Linux x64

El paquete contiene un ejecutable con la interfaz y el motor. Abre una ventana
propia mediante GTK 3 y WebKitGTK 4.1 o 4.0. Requiere esas bibliotecas y una
sesión gráfica. En Bazzite y Fedora, el paquete de WebKitGTK 4.1 se llama
`webkit2gtk4.1`. Si falta alguna dependencia, ejecuta desde la terminal para
ver el diagnóstico. **Examinar** abre el selector nativo de GTK. También
puedes pegar las rutas. Consulta la [guía del Flatpak](../packaging/flatpak/README.md)
para instalar el paquete con sus dependencias.

## macOS

Los paquetes experimentales de macOS existentes abren la interfaz en el
navegador cuando se compilan sin cgo. Usa arm64 en Apple Silicon o amd64 en
Intel. La compilación nativa con cgo requiere el SDK de Apple.

## Uso

Extrae el archivo de tu plataforma y ejecuta:

    chmod +x ./molten-core
    ./molten-core

Cierra WoW y Battle.net antes de instalar. En Linux, la copia está destinada
a Wine o Proton. **Cerrar** solicita la cancelación de cualquier operación
activa. Los comandos de terminal siguen disponibles:

    ./molten-core --validate 1.60.1.71037
    ./molten-core --target 1.60.1.71037 '/ruta/World of Warcraft' '/ruta/WoW-Forever-71037'

Para compilar el ejecutable de Linux desde el código fuente, usa Go 1.24 o
posterior y un compilador C:

    CGO_ENABLED=1 go build -trimpath -o molten-core ./cmd/molten-core
