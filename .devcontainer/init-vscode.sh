#!/bin/bash

scriptFullPath="$(realpath "$0")"
projectPath="$(dirname "$(dirname "$scriptFullPath")")"

echo "Script full path: $projectPath"

mkdir -p "$projectPath/.vscode/data/zsh"
echo "*" > "$projectPath/.vscode/.gitignore"
cat <<EOF > "$projectPath/.vscode/settings.json"
{
    "terminal.integrated.env.linux": {
        "XDG_DATA_HOME": "$projectPath/.vscode/data",
        "ZDOTDIR": "$projectPath/.vscode/data/zsh"
    },
    "terminal.integrated.defaultProfile.linux": "zsh"
}
EOF
cat <<'EOF' > "$projectPath/.vscode/data/zsh/.zshrc"
export ZSH="$HOME/.oh-my-zsh"
ZSH_THEME="ys"
plugins=(git docker kubectl)
source $ZSH/oh-my-zsh.sh
zstyle ':omz:update' mode disabled
EOF
