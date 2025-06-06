complete -c recur -s h -l help -d "Print help message and exit"
complete -c recur -s V -l version -d "Print version number and exit"
complete -c recur -s a -l attempts -x -d "Maximum number of attempts" -a "10 -1"
complete -c recur -s b -l backoff -x -d "Base for exponential backoff" -a "0 1.1s 2s"
complete -c recur -s c -l condition -x -d "Success condition" -a "'code == 0' 'code != 0'"
complete -c recur -s d -l delay -x -d "Constant delay" -a "1s 5s 30s 1m 5m"
complete -c recur -s f -l forever -d "Infinite attempts"
complete -c recur -s j -l jitter -x -d "Additional random delay" -a "1s 1s,5s 1m"
complete -c recur -s m -l max-delay -x -d "Maximum allowed delay" -a "1s 5s 30s 1m 5m"
complete -c recur -s t -l timeout -x -d "Timeout for each attempt" -a "1s 5s 30s 1m 5m"
complete -c recur -s v -l verbose -d "Increase verbosity"

# Complete with available commands.
complete -c recur -n "not __fish_seen_subcommand_from (__fish_complete_command)" -a "(__fish_complete_command)"
