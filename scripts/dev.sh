#!/usr/bin/env bash
# Run `wails dev` with a clean environment.
#
# Ubuntu's VS Code *snap* injects GTK/GLib/snap variables into its integrated
# terminal that point native apps at snap's bundled (older glibc) libraries,
# causing:
#   symbol lookup error: /snap/core20/.../libpthread.so.0: undefined symbol
#     __libc_pthread_init, version GLIBC_PRIVATE
# This wrapper unsets those variables so the Wails binary uses the system libs.
# (Running from a normal, non-snap terminal avoids the problem entirely.)
set -euo pipefail
cd "$(dirname "$0")/.."

# Ensure user-local Go + Wails are on PATH even if .bashrc wasn't sourced.
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

# Strip snap-injected environment that redirects GTK/GLib to core20 libs.
unset LD_LIBRARY_PATH LD_PRELOAD \
      GTK_PATH GTK_EXE_PREFIX GTK_IM_MODULE_FILE \
      GDK_PIXBUF_MODULE_FILE GDK_PIXBUF_MODULEDIR \
      GIO_MODULE_DIR GIO_LAUNCHED_DESKTOP_FILE \
      GSETTINGS_SCHEMA_DIR LOCPATH \
      SNAP SNAP_NAME SNAP_ARCH SNAP_INSTANCE_NAME SNAP_LIBRARY_PATH \
      SNAP_REVISION SNAP_VERSION SNAP_DATA SNAP_COMMON SNAP_USER_DATA \
      SNAP_USER_COMMON SNAP_CONTEXT SNAP_COOKIE SNAP_REAL_HOME \
      SNAP_EUID SNAP_UID SNAP_LAUNCHER_ARCH_TRIPLET 2>/dev/null || true

TAGS="${WEBKIT_TAGS:-webkit2_41}"
exec wails dev -tags "$TAGS" "$@"
