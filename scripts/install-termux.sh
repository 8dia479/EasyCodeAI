#!/data/data/com.termux/files/usr/bin/bash
set -eu

if [ -z "${PREFIX:-}" ] || [ ! -d "$PREFIX" ]; then
  echo "Run this installer inside Termux on Android." >&2
  exit 1
fi

for command in git go; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Missing dependency: $command. Install it with: pkg install $command" >&2
    exit 1
  fi
done

repo_dir="${EASYCODE_REPO_DIR:-$HOME/EasyCodeAI}"
branch="Alpha0.1"

if [ -d "$repo_dir/.git" ]; then
  git -C "$repo_dir" fetch origin "$branch"
  git -C "$repo_dir" switch "$branch"
  git -C "$repo_dir" pull --ff-only origin "$branch"
else
  git clone --depth 1 --branch "$branch" https://github.com/8dia479/EasyCodeAI.git "$repo_dir"
fi

cd "$repo_dir"
go build -trimpath -o "$PREFIX/bin/easycode" ./cmd/easycode
chmod 700 "$PREFIX/bin/easycode"

echo "EasyCodeAI installed: $PREFIX/bin/easycode"
echo "Set NIM_API_KEY and NIM_MODEL, then run: easycode --help"
