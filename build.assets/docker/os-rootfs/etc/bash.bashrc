
if [[ -f /etc/bash_completion ]]; then
    source /etc/bash_completion
fi

if [[ -f /etc/proxy-environment ]]; then
    source /etc/proxy-environment
fi

_kubectl_completion() {
    source <(kubectl completion bash)
}

_start() {
    _kubectl_completion
}

HISTCONTROL=ignoreboth
HISTSIZE=1000
HISTFILESIZE=2000
PROMPT_DIRTRIM=3

shopt -s histappend
shopt -s checkwinsize
shopt -s autocd
shopt -s nocaseglob
shopt -s cdspell
shopt -s cmdhist
shopt -s dirspell

export PS1='\t \u@\h:\w\$ '
export EDITOR="/usr/bin/vim"
export VIEWER="less"
export PAGER="less"
export SELECTED_EDITOR=$EDITOR
export HISTTIMEFORMAT="%y.%m.%d %T "

alias kctl="kubectl -nkube-system"

_start

[ ! -z "$TERM" -a -r /etc/motd ] && cat /etc/motd

# set a fancy prompt (non-color, unless we know we "want" color)
case "$TERM" in
    xterm-color) color_prompt=yes;;
esac

# nice colors for bash
C_RED="\[\033[0;31m\]"
C_GREEN="\[\033[0;32m\]"
C_LIGHT_GRAY="\[\033[0;37m\]"
C_RESET="\[\033[0m\]"

C_BROWN="\[\033[0;33m\]"
C_BLUE="\[\033[0;34m\]"
C_PURPLE="\[\033[0;35m\]"
C_CYAN="\[\033[0;36m\] "
C_GRAY="\[\033[1;30m\]"
C_WHITE="\[\033[1;37m\]"
C_YELLOW="\[\033[1;33m\]"

C_LIGHT_BLUE="\[\033[1;34m\]"
C_LIGHT_CYAN="\[\033[1;36m\]"
C_LIGHT_PURPLE="\[\033[1;35m\]"
C_LIGHT_RED="\[\033[1; 31m\]"
C_LIGHT_GREEN="\[\033[1;32m\]"

PS1="$C_BLUE\h:\w$ $C_RESET"
