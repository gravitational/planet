
if [[ -f /etc/bash_completion ]]; then
    source /etc/bash_completion
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
