# Molten Core - WoW Forever Downgrader

Instalador independiente de versiones de WoW Forever para Windows y Linux.
Descarga el ejecutable para Windows x64 o el Flatpak para Linux x64 desde [Releases](../../releases).
La interfaz está disponible en español, inglés, francés, portugués, alemán y ruso. Windows requiere Microsoft Edge WebView2 Runtime. Para Linux, consulta la [guía de instalación del Flatpak](packaging/flatpak/README.md).

1. Cierra WoW y Battle.net.
2. Selecciona como origen tu instalación de WoW Forever de Battle.net, actualizada a la versión más reciente, y una carpeta de destino separada.
3. Escribe la versión completa que necesitas. Pulsa **Validar** y luego **Instalar**.
4. La copia quedará en la subcarpeta `_forever_` del destino.

La copia es un cliente completo de la versión elegida para tu plataforma y el
idioma seleccionado: no depende de que el juego descargue datos al arrancar.
Los archivos que ya coinciden en la instalación de Battle.net se copian y
verifican localmente (el origen solo se lee, nunca se modifica); solo se
descarga del CDN lo que falta o cambió. Asegúrate de tener espacio libre
suficiente para un cliente completo en el destino.

## Actualizaciones

La edición pública de Windows consulta las Releases de este repositorio. La descarga
empieza cuando eliges **Actualizar ahora**. Solo se publican versiones
aprobadas por el propietario tras las pruebas en el repositorio privado.

## Autoría y descargo de responsabilidad

El código de Molten Core fue escrito con las IA ChatGPT (OpenAI) y Claude
(Anthropic), a partir del código de [Project Everlake](https://github.com/buildthehomelab/wow-everlake-launcher).
Es un proyecto independiente sin afiliación ni respaldo de Blizzard Entertainment.
El software se proporciona tal cual, sin garantías. Úsalo bajo tu propia
responsabilidad y consulta el alcance de la licencia en [GPL-2.0](LICENSE).

## Alcance del código fuente

Este árbol contiene el instalador de Molten Core, sus dependencias, pruebas y
scripts de compilación. Los componentes independientes del lanzador de Everlake
para iniciar sesiones y conectarse a servidores no forman parte de este árbol.

## Compilación

En Windows x64 con Go 1.24 o posterior, ejecuta:

    .\build-windows.ps1 -Version v1.9.3 -Channel public
