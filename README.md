# fpm Go Edition

Esta carpeta contiene la migración de FPM desde C a Go. El objetivo es conservar la interfaz de comandos y mejorar la mantenibilidad, la seguridad de memoria y el control explícito de las operaciones destructivas con otra eficiencia.

## Requisitos

El binario necesita ejecutarse en Linux y requiere `xbps-install` para los comandos de paquetes XBPS. Las funciones Flatpak requieren además el ejecutable `flatpak`. Las acciones que modifican el sistema necesitan ejecutarse como root o disponer de `doas`/`sudo`.

La interfaz interactiva utiliza `github.com/gdamore/tcell/v2`, que evita depender directamente de `ncurses`. Los catálogos de inglés, español, portugués, catalán, italiano y francés se incorporan al binario durante la compilación.

## Controles de búsqueda

En `fpm search <texto>` y en las vistas interactivas, la lista admite teclado y mouse. `PageUp` y `PageDown` desplazan una página completa; `Home` y `End` van al primer y último resultado. La rueda del mouse desplaza tres resultados por evento. El clic izquierdo selecciona y marca el resultado. El clic derecho desmarca únicamente el resultado situado bajo el cursor. El clic de la rueda central confirma la selección actual. El botón central no modifica las marcas de otros resultados. El filtro, el cursor y las selecciones permanecen al volver desde ese menú.

La búsqueda unificada presenta primero los resultados XBPS disponibles y abre la TUI inmediatamente. En paralelo, Flatpak y AppImage se consultan y se agregan a la misma lista conforme terminan, mostrando `cargando más fuentes...` mientras quedan fuentes pendientes. Esto permite comenzar a navegar sin esperar a todos los repositorios.

| Control | Acción |
|---|---|
| `↑`/`↓`, `j`/`k` | Mover el cursor |
| `PageUp`/`PageDown` | Retroceder o avanzar una página |
| `Home`/`End` | Ir al primer o último resultado |
| Rueda del mouse | Desplazar la lista |
| Clic izquierdo | Seleccionar y marcar |
| Clic derecho | Desmarcar |
| Clic de rueda central | Confirmar las marcas actuales |
| `Space` | Marcar/desmarcar |
| `Enter` | Confirmar selección |
| `Esc`/`q` | Salir o volver |

## Confirmaciones

FPM **no añade `-y` automáticamente** a XBPS ni a Flatpak. Las herramientas subyacentes mostrarán sus confirmaciones normales. La confirmación automática solo se activa cuando el usuario pasa explícitamente `-y` o `--yes` a FPM.

Esto aplica a instalación, eliminación, actualización, limpieza de caché, eliminación de huérfanos y operaciones equivalentes. Las acciones interactivas propias de FPM también respetan esa flag.

## Compilación y pruebas

```sh
go mod download
go test ./...
go test -race ./...
go vet ./...
make build
make check-i18n
```

Para instalarlo en `/usr/local/bin`:

```sh
sudo make install
```

Para desinstalar únicamente el ejecutable instalado por FPM (sin borrar la
base de datos oficial de XBPS ni los datos de otros gestores):

```sh
sudo make uninstall
```

`uninstall` usa el mismo `PREFIX` y `DESTDIR` que `install`, por ejemplo:

```sh
sudo make PREFIX=/usr uninstall
make DESTDIR=/tmp/pkgroot PREFIX=/usr uninstall
```

## Uso XBPS

```text
fpm install <paquete...>
fpm remove <paquete...>
fpm search <texto>
fpm info <paquete>
fpm update
fpm upgrade
fpm list [-a|-s]
fpm repair
fpm clean
fpm check
fpm files <paquete>
fpm owns <ruta>
fpm deps <paquete> [-r]
fpm hold <paquete...>
fpm unhold <paquete...>
fpm orphans
fpm autoremove
fpm size <paquete...>
fpm diskusage
fpm cache-info
fpm db list
sudo fpm db sync
fpm db check
fpm db repair
```

`cache-info` muestra el tamaño y número de archivos de `/var/cache/xbps` y `/var/db/xbps`, además del número de paquetes instalados cuando está disponible. `db check` ejecuta `xbps-pkgdb -k`, mientras que `db repair` ejecuta `xbps-pkgdb -a` y comprueba el código de salida.

FPM mantiene un snapshot/caché SQLite global propio en `/var/lib/fpm/fpm.db`
para guardar su inventario consultable (`fpm db list`). Se reconstruye desde
`xbps-query -l` con `sudo fpm db sync` y se refresca después de transacciones
XBPS exitosas realizadas por FPM. Si XBPS se modifica desde otra aplicación,
ejecuta `sudo fpm db sync`; XBPS en `/var/db/xbps` continúa siendo la única
fuente autoritativa para instalar, actualizar y resolver paquetes. `make uninstall`
elimina el binario, pero conserva el snapshot de FPM.

La base de Yelena (`~/.config/cuerdtoken/updates.db`) sigue siendo otra base
SQLite, por usuario, destinada solo a candidatos de actualización y su estado
en la interfaz. No es la base de inventario de FPM ni reemplaza a XBPS. NISSA
intercambia consultas/resultados entre ambos programas sin fusionar estas
bases.

## Comandos cortos para backends

Los tres backends se invocan con comandos breves: `fpm install <paquete...>` para XBPS, `fpm flat <acción>` para Flatpak y `fpm aimg <acción>` para AppImage. Los nombres anteriores `flatpak` y `appimage` siguen funcionando como alias compatibles.

FPM espera a que cada operación termine, incluso si tarda mucho; no cierra el backend por un límite de tiempo. La salida del gestor de paquetes se mantiene visible. Con `-v` se puede mostrar el estado de la operación, pero FPM no imprime los comandos internos usados para ejecutar los backends.

## Uso Flatpak y remotos

La instalación sin remoto explícito delega la resolución a Flatpak y no impone Flathub:

```sh
fpm flat install org.gimp.GIMP
```

Para seleccionar un remoto concreto:

```sh
fpm flat install --remote flathub org.gimp.GIMP
fpm flat install --remote gnome org.gnome.Builder
```

La búsqueda utiliza los remotos configurados y conserva el remoto de cada resultado para que una selección se instale desde su origen correcto. La administración de remotos está disponible con:

```text
fpm flat remotes
fpm flat remote-add <nombre> <url-o-flatpakrepo>
fpm flat remote-remove <nombre>
fpm flat remote-ls [nombre]
fpm flat remote-info <nombre> <application-id>
fpm flat setup-flathub
```

`setup-flathub` utiliza el repositorio oficial `https://dl.flathub.org/repo/flathub.flatpakrepo` y pide confirmación, salvo que se use `-y`. También se puede usar `--user` con los comandos Flatpak para operar sobre la instalación por usuario:

```sh
fpm flat --user remotes
fpm flat --user install --remote flathub org.gimp.GIMP
```

Flatpak permite repositorios de sistema y por usuario. FPM no sustituye la verificación criptográfica de Flatpak ni intenta reimplementar OSTree; delega esas operaciones al ejecutable oficial.

## Uso AppImage

La instalación sin opciones es interactiva y pregunta el nombre, descripción, categoría e ícono:

```sh
fpm aimg install ./MiApp.AppImage
```

Para automatizar únicamente la captura de metadatos, se pueden indicar opciones explícitas:

```sh
fpm aimg install ./MiApp.AppImage \\
  --name "Mi aplicación" \\
  --desc "Descripción de la aplicación" \\
  --category "Utility;" \\
  --icon application-x-executable
```

También se admite la forma `--opción=valor`. Para forzar el flujo interactivo se puede usar `--interactive` aunque se hayan definido opciones. Las operaciones de administración son:

```text
fpm aimg remove [nombre]
fpm aimg remove --name "Mi aplicación"
fpm aimg update ./MiApp-nueva.AppImage
fpm aimg update --name "Mi aplicación" ./MiApp-nueva.AppImage
fpm aimg list
fpm aimg list --plain
fpm aimg manage
```

| Opción | Función |
|---|---|
| `--name NAME` | Nombre que se escribe en `Name=` del lanzador `.desktop`; en `remove`/`update` identifica la aplicación. |
| `--desc TEXT` | Descripción que se escribe en `Comment=`. |
| `--category VALUE` | Categoría XDG, por ejemplo `Utility;`, `Graphics;` o `Development;`. |
| `--icon NAME\|PATH` | Nombre de ícono del tema o ruta a una imagen que FPM copiará al directorio de iconos. |
| `--plain` | Emite registros separados por tabuladores para integración con frontends. |
| `--interactive` | Fuerza las preguntas interactivas durante la instalación. |

Si se proporciona cualquier metadato durante `install`, la operación es no interactiva. Si no se proporciona ninguno, se conserva el flujo interactivo original.

## Diseño

`process.go` encapsula la ejecución directa de procesos. No se construyen comandos para un shell: cada argumento se pasa separadamente a `os/exec`, lo que evita expansiones accidentales y reduce el riesgo de inyección por entradas de usuario.

`commands.go` contiene las operaciones XBPS y la búsqueda unificada. `cache.go` encapsula las estadísticas de caché y las operaciones explícitas de base de datos. `flatpak.go` encapsula Flatpak y sus remotos. `appimage.go` gestiona copias, permisos, rutas XDG y lanzadores `.desktop`. `tui.go` contiene la selección interactiva. `display_i18n.go` centraliza los mensajes y los catálogos embebidos.

La lógica continúa usando los ejecutables oficiales `xbps-*` y `flatpak`; no intenta reimplementar sus bases de datos ni sus reglas de resolución de dependencias. Esto reduce el alcance y mantiene la compatibilidad con el sistema instalado.

## Diferencias y límites conocidos

La versión Go cambia la versión mostrada a `1.2-go` para distinguirla durante la validación. Los resultados de XBPS y Flatpak se interpretan a partir de sus salidas textuales o tabulares; si esas herramientas cambian sus formatos, los parseadores deberán actualizarse.

Antes de reemplazar la versión C en producción, conviene probar las operaciones destructivas en una instalación Void Linux de prueba y comparar sus salidas con la versión original para cada comando.

## Seguridad y mantenibilidad

La migración elimina la gestión manual de memoria del programa original: no hay `malloc/free`, buffers fijos para listas de paquetes ni construcción manual de `argv`. Las operaciones de archivos devuelven errores de Go y las pruebas cubren parseo de lanzadores, nombres, búsqueda, detección de paquetes y la regla de confirmación explícita.


## Integración con Yelena Software: NISSA v1 y v2

FPM ofrece un protocolo de llamadas estructuradas para aplicaciones hermanas como Yelena Software. NISSA (Native Inter-Software Signal API) se invoca localmente mediante subcomandos y responde con JSON Lines; no inicia un daemon, no eleva privilegios y no altera la salida habitual del CLI.

```sh
fpm nissa v1 hello
fpm nissa v1 search "firefox"
```

`hello` negocia el protocolo y anuncia capacidades. NISSA v1 se mantiene compatible y expone búsqueda; NISSA v2 añade consultas estructuradas de paquetes instalados, actualizaciones pendientes y detalles de paquete:

```sh
fpm nissa v2 hello
fpm nissa v2 search "firefox"
fpm nissa v2 installed
fpm nissa v2 updates
fpm nissa v2 info firefox
```

Los flujos JSON Lines terminan con una señal `*.done` con el recuento de registros para que el cliente rechace respuestas truncadas o incoherentes. `updates` solo consulta el índice XBPS local; la sincronización de repos y las operaciones de instalación, eliminación o actualización siguen siendo transacciones normales de FPM, no llamadas NISSA con privilegios. Tras una transacción XBPS/Flatpak exitosa, FPM avisa al applet de Yelena con `SIGUSR2` (validando usuario y proceso) para que vuelva a consultar y sincronice su base SQLite de actualizaciones. Yelena usa NISSA v2 para búsqueda, inventario y detección de actualizaciones; las operaciones iniciadas desde la GUI conservan su autorización polkit existente.

La especificación de campos, errores y ejemplos está en [`NISSA.md`](NISSA.md). Las pruebas de parseo y validación del protocolo se ejecutan con `go test ./...`.
