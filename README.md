# fpm (Fast Package Manager)

Un gestor de paquetes interactivo en terminal diseñado específicamente para **XBPS** (X Binary Package System). **fpm** combina la velocidad de los componentes de bajo nivel desarrollados en **C** con una interfaz visual fluida y ligera construida sobre **curses**.

Ideal para entornos minimalistas basados en CLI/TUI donde se requiere eficiencia extrema sin perder la comodidad de una interfaz navegable.

## Requisitos de Dependencias

Antes de compilar e instalar, asegúrate de contar con los siguientes paquetes en tu sistema Void:

- `base-devel` (para compilar los componentes en C)
- `ncurses-devel` (o las librerías de `curses` correspondientes)
- `xbps` (sistema nativo de gestión de paquetes)


## Instalación y Compilación

Para compilar las extensiones nativas en C y preparar el entorno de ejecución, ejecuta:

```bash
# Clonar el repositorio
git clone [https://github.com/CuerdOS-Project/fpm.git](https://github.com/CuerdOS-Project/fpm.git)
cd fpm

# Compilar los componentes nativos en C
make

# Ejecutar de forma local
./fpm

```

## Uso

Una vez iniciado el gestor, puedes interactuar mediante atajos de teclado estándar de entornos de terminal:

```bash
Uso: fpm <comando> [argumentos...]

Comandos:
  install <pkg...>       Instalar paquete(s)
  remove <pkg...>        Eliminar paquete(s)
  superremove <pkg...>   Eliminación profunda + limpieza
  search <query>         Buscar paquetes
  info <pkg>             Información de un paquete
  update                  Sincronizar repositorios
  upgrade                 Actualizar el sistema
  list [-a|-s]            Listar paquetes instalados
  repair                  Reparar el sistema de paquetes
  clean                   Limpiar caché de paquetes descargados
  check                   Verificar integridad del sistema de paquetes
  files <pkg>             Listar archivos instalados por un paquete
  owns <path>             Buscar qué paquete provee un archivo
  deps <pkg> [-r]         Mostrar dependencias de un paquete
  hold <pkg...>           Retener paquete(s) para que no se actualicen
  unhold <pkg...>         Liberar paquete(s) retenido(s)
  orphans                 Listar paquetes huérfanos
  autoremove              Eliminar paquetes huérfanos automáticamente
  size <pkg...>           Mostrar tamaño instalado de paquete(s)
  diskusage               Mostrar uso de disco de la caché de paquetes

Opciones:
  -y, --yes             Confirmar automáticamente (sin preguntar)
  -a, --all             Mostrar todos los resultados
  -s, --select          Modo de selección interactiva
  -r, --reverse         Mostrar en orden inverso
  -v, --verbose         Mostrar los comandos ejecutados
  -V, --version         Mostrar la versión de FPM
  -h, --help            Mostrar esta ayuda

```

## Licencia

Este proyecto está bajo la Licencia **GNU GPLv3** - consulta el archivo `LICENSE` para más detalles.

### Notas sobre la adaptación:
- **Componentes C:** Se ha incluido una sección de compilación (`Makefile`) y estructura que asume la existencia de extensiones binarias que aceleran la interacción con `xbps-query` y el procesamiento de texto.
- **Curses:** Se enfoca la sección de uso en los comandos rápidos por teclado propios de las aplicaciones hechas con dicha librería gráfica de terminal.
