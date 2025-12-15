complete -c recur -s h -l help -d "Print help message and exit"
complete -c recur -s V -l version -d "Print version number and exit"
complete -c recur -s a -l attempts -d "Maximum number of attempts" -a "10 -1" -x
complete -c recur -s b -l backoff -d "Base for exponential backoff" -a "0 1.1s 2s" -x
complete -c recur -s c -l condition -d "Success condition" -a "'code == 0' 'code != 0'" -x
complete -c recur -s d -l delay -d "Constant delay" -a "1s 5s 30s 1m 5m" -x
complete -c recur -s E -l hold-stderr -d "Buffer standard error for each attempt and only print it on success"
complete -c recur -s F -l fib -d "Add Fibonacci backoff"
complete -c recur -s f -l forever -d "Infinite attempts"
complete -c recur -s I -l replay-stdin -d "Replay standard input on each attempt"
complete -c recur -s j -l jitter -d "Additional random delay" -a "1s 1s,5s 1m" -x
complete -c recur -s m -l max-delay -d "Maximum allowed delay" -a "1s 5s 30s 1m 5m" -x
complete -c recur -s O -l hold-stdout -d "Buffer standard output for each attempt and only print it on success"
complete -c recur -s R -l report -d "Report output" -a "- report.json report.txt json:- text:-" -r
complete -c recur -s r -l reset -d "Minimum attempt time that resets exponential and Fibonacci backoff" -a "5s 1m 5m 30m 1h" -x
complete -c recur -s s -l seed -d "Random seed for jitter" -a "0 123" -x
complete -c recur -s t -l timeout -d "Timeout for each attempt" -a "1s 5s 30s 1m 5m" -x
complete -c recur -s v -l verbose -d "Increase verbosity"

# Complete with available commands.
complete -c recur -n "not __fish_seen_subcommand_from (__fish_complete_command)" -a "(__fish_complete_command)"
