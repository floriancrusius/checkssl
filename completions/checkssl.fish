# fish completion for checkssl
#
# Install to:  /usr/local/share/fish/vendor_completions.d/checkssl.fish
# Or in-shell: checkssl completion fish | source

complete -c checkssl -s d -l domain      -x -d 'Domain to check (repeatable)'
complete -c checkssl -s f -l file        -r -F -d 'File with one domain per line'
complete -c checkssl -s s -l silent            -d 'Suppress error output'
complete -c checkssl      -l format      -x -a 'table csv json nagios html' -d 'Output format'
complete -c checkssl      -l concurrency -x    -d 'Max parallel TLS handshakes'
complete -c checkssl      -l timeout     -x    -d 'Per-domain handshake timeout'
complete -c checkssl      -l nagios-warning  -x -d 'Warn threshold in days (nagios only)'
complete -c checkssl      -l nagios-critical -x -d 'Critical threshold in days (nagios only)'
complete -c checkssl -s h -l help              -d 'Show help'
complete -c checkssl -s v -l version           -d 'Show version'

# Sub-command
complete -c checkssl -n '__fish_use_subcommand' -a completion -d 'Print shell completion script'
complete -c checkssl -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish' -d 'Target shell'
