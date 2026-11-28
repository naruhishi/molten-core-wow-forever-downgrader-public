# Molten Core para Bazzite

El instalador abre su propia ventana desde el menú de aplicaciones. Usa GTK y
WebKitGTK del entorno GNOME de Flatpak. La primera instalación puede descargar
el entorno desde Flathub.

Con Flathub disponible, instala y abre el paquete:

    flatpak install --user ./Molten-Core-v1.9.4-Linux-x64.flatpak
    flatpak run io.github.naruhishi.MoltenCore

El paquete tiene acceso a la red para descargar el cliente y a tu carpeta
personal y /run/media para leer el juego y guardar la copia. Elige rutas
dentro de esas ubicaciones. Puedes escribir o pegar las rutas si el selector
de carpetas no está disponible. En Bazzite, abre la copia con Wine o Proton.

Para construir el paquete a partir del código en Linux x64 con Go, un
compilador C y Flatpak:

    CGO_ENABLED=1 go build -trimpath -o molten-core ./cmd/molten-core
    bash packaging/flatpak/package.sh ./molten-core Molten-Core-v1.9.4-Linux-x64.flatpak
