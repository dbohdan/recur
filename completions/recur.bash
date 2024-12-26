_recur() {
    local cur prev opts

    COMPREPLY=()
    cur=${COMP_WORDS[COMP_CWORD]}
    prev=${COMP_WORDS[COMP_CWORD - 1]}
    opts='-h --help -V --version -a --attempts -b --backoff -c --condition -d --delay -f --forever -j --jitter -m --max-delay -t --timeout -v --verbose'

    case "${prev}" in
    -a | --attempts)
        COMPREPLY=($(compgen -W "10 -1" -- ${cur}))
        return 0
        ;;
    -b | --backoff)
        COMPREPLY=($(compgen -W "1.1s 2s" -- ${cur}))
        return 0
        ;;
    -c | --condition)
        COMPREPLY=($(compgen -W "'code==0' 'code!=0'" -- ${cur}))
        return 0
        ;;
    -d | --delay | -m | --max-delay | -t | --timeout)
        COMPREPLY=($(compgen -W "1s 5s 30s 1m 5m" -- ${cur}))
        return 0
        ;;
    -j | --jitter)
        COMPREPLY=($(compgen -W "1s 1s,5s 1m" -- ${cur}))
        return 0
        ;;
    esac

    if [[ ${cur} == -* ]]; then
        COMPREPLY=($(compgen -W "${opts}" -- ${cur}))
        return 0
    fi

    # Complete with commands if no other completion applies.
    COMPREPLY=($(compgen -c -- "${cur}"))
}

complete -F _recur recur
