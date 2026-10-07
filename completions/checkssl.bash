# bash completion for checkssl
# shellcheck shell=bash
#
# Install to:  /usr/local/etc/bash_completion.d/checkssl                (macOS, Homebrew)
#              /usr/local/share/bash-completion/completions/checkssl     (Linux)
# Or in-shell: eval "$(checkssl completion bash)"

_checkssl() {
    local cur prev flags formats statuses
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    flags='-d --domain -f --file -s --silent --format --concurrency --timeout --nagios-warning --nagios-critical --only -h --help -v --version'
    formats='table csv json nagios html'
    statuses='valid expiring_soon expired invalid error'

    case "$prev" in
        --format)
            # shellcheck disable=SC2207
            COMPREPLY=( $(compgen -W "$formats" -- "$cur") )
            return 0
            ;;
        --only)
            # shellcheck disable=SC2207
            COMPREPLY=( $(compgen -W "$statuses" -- "$cur") )
            return 0
            ;;
        -f|--file)
            # shellcheck disable=SC2207
            COMPREPLY=( $(compgen -f -- "$cur") )
            return 0
            ;;
        -d|--domain|--concurrency|--timeout|--nagios-warning|--nagios-critical)
            return 0
            ;;
    esac

    if [[ "$cur" == -* ]]; then
        # shellcheck disable=SC2207
        COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
    fi
}

complete -F _checkssl checkssl
