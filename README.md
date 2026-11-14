# fpm Go Edition — 2.2

Esta carpeta contiene la migración de FPM desde C a Go. El objetivo es conservar la interfaz de comandos y mejorar la mantenibilidad, la seguridad de memoria y el control explícito de las operaciones destructivas con otra eficiencia.

## Estructura del código

```text
cmd/fpm/          Punto de entrada mínimo del ejecutable
internal/fpm/     Implementación interna y catálogos embebidos
  *_test.go       Pruebas unitarias del paquete (acceden a detalles internos)
tests/            Pruebas de integración del ejecutable
```

Las pruebas unitarias permanecen junto al paquete que prueban, como recomienda Go para acceder a funciones internas; las pruebas de integración de CLI están separadas en `tests/`. `go test ./...` ejecuta ambas.

## Requisitos

El binario necesita ejecutarse en Linux y requiere `xbps-install` para los comandos de paquetes XBPS. Las funciones Flatpak requieren además el ejecutable `flatpak`. Las acciones que modifican el sistema necesitan ejecutarse como root o disponer de `doas`/`sudo`.

La interfaz interactiva utiliza `github.com/gdamore/tcell/v2`, que evita depender directamente de `ncurses`. Los catálogos de inglés, español, portugués, catalán, italiano y francés se incorporan al binario durante la compilación.

## Controles de búsqueda

En `fpm search <texto>` y en las vistas interactivas, la lista admite teclado y mouse. `PageUp` y `PageDown` desplazan una página completa; `Home` y `End` van al primer y último resultado. La rueda del mouse desplaza tres resultados por evento. El clic izquierdo selecciona y marca el resultado. El clic derecho desmarca únicamente el resultado situado bajo el cursor. El clic de la rueda central confirma la selección actual. El botón central no modifica las marcas de otros resultados. El filtro, el cursor y las selecciones permanecen al volver desde ese menú.

La búsqueda unificada presenta primero los resultados XBPS disponibles y abre la TUI inmediatamente. En paralelo, Flatpak y AppImage se consultan y se agregan a la misma lista conforme terminan, mostrando `cargando más fuentes...` mientras quedan fuentes pendientes. Esto permite comenzar a navegar sin esperar a todos los repositorios.

| Control | Acción |
|---|---|
| `↑`/`↓` | Mover el cursor |
| `PageUp`/`PageDown` | Retroceder o avanzar una página |
| `Home`/`End` | Ir al primer o último resultado |
| Rueda del mouse | Desplazar la lista |
| Clic izquierdo | Seleccionar y marcar |
| Clic derecho | Desmarcar |
| Clic de rueda central | Confirmar las marcas actuales |
| `Space` | Marcar/desmarcar |
| `Ctrl+A` | Marcar/desmarcar todo lo visible |
| `Tab` / `Shift+Tab` | Cambiar de fuente (Todas → XBPS → Flatpak → am-FUSION → AppImage) |
| Clic en una pestaña de fuente | Filtrar por esa fuente |
| `Enter` | Confirmar selección |
| `Esc` / `Ctrl+C` | Salir o volver |

Las letras siempre se escriben en el filtro (incluidas `a` y `q`); para salir se usa `Esc` y para marcar todo `Ctrl+A`.

### Filtrar por fuente

Cada resultado lleva su fuente (`XBPS`, `Flatpak`, `am-FUSION`, `AppImage`) en una etiqueta de color, y bajo el filtro hay una pestaña por fuente con el número de resultados que coinciden con el texto escrito. El filtro y los contadores se calculan en un solo recorrido, sin reservar memoria por elemento, y solo cuando cambia el texto o llegan resultados nuevos.

La fuente también puede escribirse en el propio filtro (en la TUI `Space` marca paquetes, así que se une al texto con `:`):

```text
@flatpak:firefox          solo Flatpak
@xbps,am:browser          varias fuentes
@appimage                 solo AppImage locales
```

Nombres admitidos: `xbps`, `flatpak`, `am` (o `am-fusion`), `appimage` (o `aimg`, `local`); se acepta un prefijo si no es ambiguo (`@fl`). `Tab` y el clic reescriben esa marca por ti.

En la línea de comandos:

```sh
fpm search firefox                        # todas las fuentes
fpm search --source flatpak firefox       # solo Flatpak
fpm search --source xbps,am firefox
fpm search @flatpak firefox
```

Las fuentes excluidas **no se consultan**: con `--source flatpak` no se ejecuta `xbps-query` ni se abre AM. Las fuentes restantes se consultan en paralelo. La salida no interactiva incluye ahora también Flatpak y AppImage locales, agrupada por fuente.

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
fpm repos
fpm repos fallback status
sudo fpm repos fallback enable
sudo fpm repos fallback disable
sudo fpm repos enable nonfree
sudo fpm repos enable multilib
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

`fpm search <texto>` incluye los paquetes de XBPS y, cuando `am-FUSION` está habilitado y AM/AppMan está disponible, añade resultados AppImage de AM. En la búsqueda interactiva se pueden seleccionar para instalación junto a los resultados XBPS o Flatpak; en salida no interactiva también se imprimen los resultados AM. Las aplicaciones gráficas pueden consumir los mismos resultados mediante PLUTO v2.

`fpm update` sincroniza los índices de repositorio y, al terminar, lista automáticamente los paquetes con actualizaciones pendientes sin cambiar el sistema; `fpm upgrade` aplica las actualizaciones mediante XBPS.

`fpm repos` muestra los repositorios registrados en XBPS (`xbps-query -L`), que son los que FPM utiliza para buscar, consultar e instalar paquetes. `fpm repos enable nonfree`, `multilib`, `multilib-nonfree` o `debug` instala el paquete oficial `void-repo-*` correspondiente y después conviene ejecutar `fpm update`. No se activa ningún subrepositorio automáticamente. `current` es el repositorio principal definido por la instalación; `musl` es la variante de libc y arquitectura instalada (`current/musl`), no un subrepositorio que se cambie con un toggle. Multilib solo está disponible para x86_64 con glibc; Void no ofrece multilib en musl.

`sudo fpm repos fallback enable` agrega, de forma opt-in, las URL equivalentes de `repo-default.voidlinux.org` para los repositorios oficiales Void que ya están activos. El archivo administrado `/etc/xbps.d/zzzz-fpm-void-fallback.conf` se ordena después de los repositorios existentes: XBPS conserva la prioridad configurada y solo puede recurrir al endpoint global si el mirror principal no está disponible. `sudo fpm repos fallback disable` elimina únicamente ese archivo administrado por FPM; no toca otras fuentes. Usa `fpm repos fallback status` para consultar el estado. Después de habilitarlo, ejecuta `fpm update` para sincronizar los índices.

Esta opción **no fuerza el mirror más rápido** cuando el configurado responde: prioriza mantener intacto el orden de XBPS. `repo-default.voidlinux.org` puede dirigir a cualquiera de los mirrors Tier 1 de Void, pero no garantiza la menor latencia para cada equipo. El fallback solo se crea para URLs reconocidas como mirrors oficiales Void y rutas `current` activas; no añade repositorios CuerdOS ni activa `nonfree`, `multilib` o `musl` por sí mismo. XBPS permite varias entradas `repository=` en su configuración. Referencias: [Void Handbook — repositories](https://docs.voidlinux.org/xbps/repositories/index.html), [Void mirror list](https://xmirror.voidlinux.org/), [xbps.d(5)](https://man.voidlinux.org/xbps.d.5), [xbps-install(1)](https://man.voidlinux.org/xbps-install.1).

FPM mantiene un snapshot/caché SQLite global propio en `/var/lib/fpm/fpm.db`
para guardar su inventario consultable (`fpm db list`). Se reconstruye desde
`xbps-query -l` con `sudo fpm db sync` y se refresca después de transacciones
XBPS exitosas realizadas por FPM. Si XBPS se modifica desde otra aplicación,
ejecuta `sudo fpm db sync`; XBPS en `/var/db/xbps` continúa siendo la única
fuente autoritativa para instalar, actualizar y resolver paquetes. `make uninstall`
elimina el binario, pero conserva el snapshot de FPM.

La base de Yelena (`~/.config/cuerdtoken/updates.db`) sigue siendo otra base
SQLite, por usuario, destinada solo a candidatos de actualización y su estado
en la interfaz. No es la base de inventario de FPM ni reemplaza a XBPS. PLUTO
intercambia consultas/resultados entre ambos programas sin fusionar estas
bases.

## Comandos cortos para backends

Los backends se invocan con comandos breves: `fpm install <paquete...>` para XBPS, `fpm flat <acción>` para Flatpak y `fpm aimg <acción>` para las aplicaciones AppImage. `fpm am <opciones...>` se conserva como alias de compatibilidad, pero el flujo AppImage unificado se administra desde `aimg` y el repositorio integrado `am-FUSION`. Los nombres anteriores `flatpak` y `appimage` siguen funcionando como alias compatibles.

FPM espera a que cada operación termine, incluso si tarda mucho; no cierra el backend por un límite de tiempo. La salida del gestor de paquetes se mantiene visible. Con `-v` se puede mostrar el estado de la operación, pero FPM no imprime los comandos internos usados para ejecutar los backends.

### AM/AppMan y el repositorio am-FUSION

FPM ofrece AM como fuente AppImage dentro de `aimg`, no como un segundo frontend. Registra el repositorio `am-FUSION` en su propia base SQLite, consulta el catálogo y el inventario mediante AM/AppMan y delega en ese backend la instalación, la actualización y la eliminación. AM conserva la autoridad sobre sus scripts y archivos instalados; FPM no escribe directamente en la base interna de AM.

```sh
fpm aimg repo list
fpm aimg repo add am-FUSION
# En una máquina sin AM/AppMan, elegir backend interactivo o especificarlo:
# fpm aimg repo install am-FUSION --backend appman
# fpm aimg repo install am-FUSION --backend am
fpm aimg repo sync am-FUSION
fpm aimg search firefox --repo am-FUSION
fpm aimg install --repo am-FUSION firefox --user
fpm aimg list --repo am-FUSION
fpm aimg info --repo am-FUSION firefox
fpm aimg update --repo am-FUSION firefox
fpm aimg remove --repo am-FUSION firefox
```

También se acepta `--sandbox`, `--icons`, `--debug`, `--force-latest` y `--assume-yes`/`-y` en las operaciones compatibles; FPM solo los reenvía a AM. `aimg search` sin `--repo` consulta am-FUSION además de los AppImages locales; `aimg list` combina los inventarios locales y los instalados por AM. Si `install` recibe un nombre en vez de una ruta local, usa am-FUSION; `remove` y `update` resuelven primero instalaciones locales y luego paquetes AM. `--repo am-FUSION` fuerza explícitamente esa fuente. `aimg repo disable am-FUSION` desactiva la fuente y `aimg repo enable am-FUSION` la reactiva.

`repo add` y `repo install` comprueban el backend e instalan AM si falta, y luego sincronizan el catálogo. FPM descarga el asset oficial [AM-INSTALLER 10.6](https://github.com/ivan-hc/AM/releases/download/10.6/AM-INSTALLER) y verifica el SHA-256 publicado (`ecb75ab49e6bccbf448c889c029cdd5f9234908f24054638fe2d6db9dada2d07`) antes de ejecutarlo. Con una terminal interactiva, el instalador oficial ofrece AM de sistema o AppMan local; en una ejecución no interactiva se requiere `--backend am` o `--backend appman`. La selección explícita se guarda en la base de am-FUSION y las instalaciones GUI prefieren AppMan en modo de usuario; solo una elección explícita de `--backend am` dirige las operaciones al AM de sistema, que puede requerir `sudo`/`doas`. El instalador 10.6 descarga a su vez `INSTALL` o `APP-MANAGER` desde la rama `main` de AM, tal como define upstream.

Todos los registros AppImage de FPM —locales y gestionados por AM— se guardan en un solo archivo SQLite por usuario: `$XDG_DATA_HOME/fpm/am-fusion.db` (por defecto `~/.local/share/fpm/am-fusion.db`), en la tabla `appimage_packages`, separados por la fuente `FPM-local` o `am-FUSION`. No hay una base FPM distinta para cada fuente. El otro archivo de FPM, `/var/lib/fpm/fpm.db`, es únicamente el caché de paquetes XBPS. AM conserva su propio inventario interno como fuente de verdad; FPM no escribe directamente en esa base y refresca su espejo mediante `am -fi --byname`. `repo sync` obtiene el catálogo con `am -l --appimages`.

La interfaz delegada sigue los comandos y formatos de [AM 10.6](https://github.com/ivan-hc/AM/tree/10.6): [instalador AM-INSTALLER](https://github.com/ivan-hc/AM/blob/10.6/AM-INSTALLER), [instalar AppImages](https://github.com/ivan-hc/AM/blob/10.6/docs/guides-and-tutorials/install-appimage.md), [consultar catálogo y búsqueda](https://github.com/ivan-hc/AM/blob/10.6/docs/guides-and-tutorials/list-and-query.md) y [listar apps instaladas](https://github.com/ivan-hc/AM/blob/10.6/docs/guides-and-tutorials/files.md).

El antiguo `fpm am <opciones...>` permanece como alias compatible y reenvía las opciones originales a AM/AppMan. El resto de FPM, incluido `fpm install <paquete>`, continúa usando XBPS y no se redirige silenciosamente a AM.

## Autocompletado con Tab

```sh
fpm completion bash > ~/.local/share/bash-completion/completions/fpm
fpm completion zsh  > ~/.zfunc/_fpm            # con ~/.zfunc en $fpath
fpm completion fish > ~/.config/fish/completions/fpm.fish
```

`sudo make install` instala los tres scripts en `$(PREFIX)/share`. Se completan comandos y opciones, subcomandos (`db`, `repos`, `kernel`, `aimg`, `flat`, `pluto`), paquetes instalados (`remove`, `hold`, `files`, `size`, `deps`, `info`), paquetes del repositorio (`install`, desde 2 caracteres), versiones de kernel instaladas (`kernel rm`), remotos y apps de Flatpak, apps AppImage, `--repo`/`--backend` y rutas de archivo (`owns`, `aimg install`). El script llama a `fpm __complete`, un comando interno silencioso que no requiere privilegios.

## Resultados XBPS repetidos

`fpm search`, la búsqueda interactiva y PLUTO (v1 y v2) muestran un solo resultado por paquete cuando varios repositorios lo ofrecen (`foo-1.0_1` y `foo-1.1_1` cuentan como el mismo). Se conserva el instalado; si no, el de repositorio de mayor prioridad (`current`, `cuerdos`, `multilib`, `nonfree`; `xbps-query` lista en el orden configurado) y, por último, el que trae más datos. Es la misma regla que `backend/dedupe.py` de Yelena Software.

## Kernels

Solo Void Linux. El inventario se lee de `/boot`, `/usr/lib/modules` y XBPS, así que **no requiere `vkpurge`**. Cada kernel se marca como en uso, perteneciente a un paquete `linuxX.Y` o sobrante (versión antigua de un paquete ya actualizado).

```text
fpm kernel                      # = fpm kernel list
fpm kernel list                 # kernel en uso, kernels de paquete y sobrantes
fpm kernel current              # versión del kernel en uso (uname -r)
sudo fpm kernel rm <versión...> # sobrante: vkpurge rm (o nativo); de paquete: xbps-remove linuxX.Y
sudo fpm kernel rm all          # todos los sobrantes (no toca kernels de paquetes)
```

`fpm kernels` es un alias. El kernel en uso nunca se elimina, las versiones se validan (`6.6.30_1` o `all`) y se pide confirmación salvo que se pase `-y`/`--yes`. Sin `vkpurge`, los sobrantes se eliminan de forma nativa (`/boot/{vmlinuz,initramfs,config,System.map}-<versión>` y `/usr/lib/modules/<versión>`) ejecutando los hooks de `/etc/kernel.d/{pre,post}-remove` con el paquete (`linuxX.Y`) y la versión como argumentos. `list` y `current` no requieren privilegios; `rm` se eleva como el resto de operaciones de escritura.

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
fpm aimg repo list
fpm aimg repo sync am-FUSION
fpm aimg search firefox --repo am-FUSION
fpm aimg install --repo am-FUSION firefox
fpm aimg remove --repo am-FUSION firefox
fpm aimg update --repo am-FUSION firefox
```

| Opción | Función |
|---|---|
| `--name NAME` | Nombre que se escribe en `Name=` del lanzador `.desktop`; en `remove`/`update` identifica la aplicación. |
| `--desc TEXT` | Descripción que se escribe en `Comment=`. |
| `--category VALUE` | Categoría XDG, por ejemplo `Utility;`, `Graphics;` o `Development;`. |
| `--icon NAME\|PATH` | Nombre de ícono del tema o ruta a una imagen que FPM copiará al directorio de iconos. |
| `--plain` | Emite registros separados por tabuladores para integración con frontends. |
| `--interactive` | Fuerza las preguntas interactivas durante la instalación. |
| `--repo am-FUSION` | Usa AM/AppMan como backend para consultar o gestionar un AppImage del repositorio FUSION. |
| `--backend` | Solo con `aimg repo add/install`; acepta `am` para instalación de sistema o `appman` para instalación local. |

El gestor admite `fpm aimg repo {list|add|install|enable|disable|sync} [am-FUSION] [--backend am|appman]`. `add` e `install` preparan el backend y sincronizan el catálogo; `enable` solo reactiva una fuente ya registrada. En las instalaciones delegadas, AM/AppMan sigue controlando los archivos y scripts de cada programa.

Si se proporciona cualquier metadato durante la instalación de un archivo local, la operación es no interactiva. Si no se proporciona ninguno, se conserva el flujo interactivo original.

## Elevación de privilegios y el helper de Yelena Software

Cuando Yelena Software ejecuta FPM con su `sudo` falso (broker `pkexec` único por sesión, directorio `yelena-aimg-*` al inicio de `PATH`), FPM lo detecta y usa ese `sudo` aunque `doas` esté instalado; así se reutiliza una sola autorización. `FPM_ELEVATOR=sudo|doas` fuerza la herramienta. Pensando en equipos que llevan meses encendidos, el helper está endurecido contra fallos de larga duración: el marcador `applet_updating.flag` se retira también cuando FPM termina por `SIGINT`/`SIGTERM`/`SIGHUP` (y la señal se reenvía con su disposición por defecto, así que el código de salida no cambia); un marcador huérfano se reemplaza aunque su PID haya sido reciclado por otro programa o sea un zombi (solo cuenta como dueño vivo un proceso con el mismo nombre que FPM); los archivos PID del usuario se leen sin seguir enlaces simbólicos, sin bloquearse con una FIFO y con un tamaño máximo, porque FPM los lee como root; y la reejecución bajo `sudo`/`doas` usa siempre una ruta absoluta del binario, incluso si el paquete se reemplazó mientras corría. Con el broker, `SUDO_UID`/`SUDO_USER` identifican al usuario invocador: FPM avisa al applet de ese usuario (`SIGUSR2`) y crea el marcador `applet_updating.flag` en su `~/.config/yl-soft` (sin seguir enlaces simbólicos y asignado a ese usuario), no en `/root`.

## Diseño

`internal/fpm/process.go` encapsula la ejecución directa de procesos. No se construyen comandos para un shell: cada argumento se pasa separadamente a `os/exec`, lo que evita expansiones accidentales y reduce el riesgo de inyección por entradas de usuario.

`internal/fpm/commands.go` contiene las operaciones XBPS y la búsqueda unificada. `cache.go` encapsula las estadísticas de caché y las operaciones explícitas de base de datos. `flatpak.go` encapsula Flatpak y sus remotos. `appimage.go` gestiona copias, permisos, rutas XDG y lanzadores `.desktop`. `tui.go` contiene la selección interactiva. `display_i18n.go` centraliza los mensajes y los catálogos embebidos en `internal/fpm/locales/`.

La lógica continúa usando los ejecutables oficiales `xbps-*` y `flatpak`; no intenta reimplementar sus bases de datos ni sus reglas de resolución de dependencias. Esto reduce el alcance y mantiene la compatibilidad con el sistema instalado.

## Diferencias y límites conocidos

Esta versión se identifica como `2.2` (estable; fuera de beta). **Cambio de protocolo:** el protocolo de comunicación NISSA fue reemplazado por PLUTO (mismas señales y semántica; el comando pasa de `fpm nissa` a `fpm pluto` y el campo `protocol` de `NISSA` a `PLUTO`), por lo que requiere Yel-Soft 1.3 o posterior. AM conserva tamaño real de descargas AppImage en bytes, expone metadatos e icono oficial en PLUTO v2 y, con `FPM_AIMG_EVENTS=1`, emite eventos JSON de progreso/instalación para operaciones delegadas de instalación y actualización. Integra los resultados de am-FUSION en `fpm search` y PLUTO v2 con etiquetas de gestor, y permite instalar resultados AM seleccionados desde la búsqueda interactiva. Mantiene `fpm aimg` como interfaz AppImage y el bootstrap verificado con AM-INSTALLER 10.6; los inventarios locales y AM comparten una sola SQLite de FPM. Los errores del CLI retornan códigos no cero y el frontend AM/AppMan elegido al instalar `am-FUSION` queda guardado: AppMan (modo de usuario) se prefiere por defecto para operaciones GUI, evitando seleccionar accidentalmente el AM global que puede solicitar `sudo`. `fpm am ...` se conserva como alias. Además, `fpm update` sincroniza los índices y lista automáticamente los paquetes pendientes (consulta de solo lectura) y `fpm upgrade` aplica las actualizaciones. `fpm repos` inspecciona repositorios XBPS registrados y permite habilitar subrepositorios oficiales. Los resultados de XBPS, Flatpak y el inventario de AM se interpretan a partir de sus salidas textuales o tabulares; si las herramientas cambian sus formatos, los parseadores deberán actualizarse.

Antes de reemplazar la versión C en producción, conviene probar las operaciones destructivas en una instalación Void Linux de prueba y comparar sus salidas con la versión original para cada comando.

## Seguridad y mantenibilidad

La migración elimina la gestión manual de memoria del programa original: no hay `malloc/free`, buffers fijos para listas de paquetes ni construcción manual de `argv`. Las operaciones de archivos devuelven errores de Go y las pruebas cubren parseo de lanzadores, nombres, búsqueda, detección de paquetes y la regla de confirmación explícita.


## Integración con Yelena Software: PLUTO v1 y v2

FPM ofrece un protocolo de comunicación de llamadas estructuradas, PLUTO, para aplicaciones hermanas como Yelena Software. PLUTO se invoca localmente mediante subcomandos y responde con JSON Lines; no inicia un daemon, no eleva privilegios y no altera la salida habitual del CLI.

```sh
fpm pluto v1 hello
fpm pluto v1 search "firefox"
```

`hello` negocia el protocolo y anuncia capacidades. PLUTO v1 se mantiene compatible y expone la búsqueda XBPS; PLUTO v2 agrega resultados AM cuando `am-FUSION` está activo e identifica cada resultado por gestor (`xbps` o `am`). También añade consultas estructuradas de paquetes instalados, actualizaciones pendientes y detalles de paquete:

```sh
fpm pluto v2 hello
fpm pluto v2 search "firefox"
fpm pluto v2 installed
fpm pluto v2 updates
fpm pluto v2 info firefox
```

Los resultados AM de `search.result` incluyen `manager: "am"`, `repository: "am-FUSION"`, y cuando están disponibles `version`, `size_bytes` e `icon_url` del catálogo oficial. En `fpm aimg list --plain`, las filas AM tienen cinco campos tabulados: `name`, `version`, `package_type`, `repository` y `size_bytes`; las filas locales conservan su formato local existente.

Para una GUI que necesite progreso en tiempo real, `FPM_AIMG_EVENTS=1` habilita mensajes opt-in con prefijo `FPM_AIMG_EVENT\t` durante instalaciones/actualizaciones AM. Cada mensaje es una línea JSON del protocolo `fpm-aimg-v1`; `event: "download"` reporta el nombre, la versión que observa AM y `downloaded_bytes`, y `event: "installed"` reporta versión y `size_bytes` finales del inventario. El total descargable puede no estar disponible, por lo que el cliente debe mostrar bytes observados y mantener avance indeterminado en vez de inventar un porcentaje. Sin esa variable, la salida CLI habitual no cambia.

Los flujos JSON Lines terminan con una señal `*.done` con el recuento de registros para que el cliente rechace respuestas truncadas o incoherentes. En búsqueda v2, `manager` y `repository` indican cómo enrutar un resultado AM; PLUTO no instala aplicaciones, por lo que la GUI debe iniciar una transacción FPM normal con confirmación. `updates` solo consulta el índice XBPS local; la sincronización de repos y las operaciones de instalación, eliminación o actualización siguen siendo transacciones normales de FPM, no llamadas PLUTO con privilegios. Durante `fpm upgrade`, FPM crea temporalmente `~/.config/yl-soft/applet_updating.flag`; el applet muestra un solo aviso de estado y anima el icono sin emitir notificaciones periódicas. El marcador se elimina al cerrar la transacción. Tras una transacción XBPS/Flatpak exitosa, FPM avisa al applet de Yelena con `SIGUSR2` (validando usuario y proceso) para que vuelva a consultar y sincronice su base SQLite de actualizaciones. Yelena usa PLUTO v2 para búsqueda, inventario y detección de actualizaciones; las operaciones iniciadas desde la GUI conservan su autorización polkit existente.

La especificación de campos, errores y ejemplos está en [`PLUTO.md`](PLUTO.md). Las pruebas de parseo y validación del protocolo se ejecutan con `go test ./...`.

### Pruebas de estrés

`internal/fpm/stress_test.go` alimenta los caminos de datos masivos (parsers de `xbps-query`, deduplicado, filtro de la TUI, SQLite, catálogo AM-FUSION y los comandos `pluto v2 installed/updates` con un `xbps` simulado) y comprueba tiempo y corrección:

```
go test ./internal/fpm -run Stress -v                 # N = 5000
FPM_STRESS_N=50000 go test ./internal/fpm -run Stress -v
FPM_STRESS=1 go test ./internal/fpm -run Stress -v    # N = 200000
```
