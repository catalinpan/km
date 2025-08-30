package completion

import (
	"fmt"
	"os"
)

// Exported variable for testing
var Exit = func(code int) { os.Exit(code) }

func Handle(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: km completion [bash|zsh|fish]")
		Exit(1)
		return
	}

	shell := args[2]
	switch shell {
	case "bash":
		fmt.Print(bashScript)
	case "zsh":
		fmt.Print(zshScript)
	case "fish":
		fmt.Print(fishScript)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported shell type %q\n", shell)
		Exit(1)
	}
}

const bashScript = `_km_completion() {
    local cur prev words cword
    _init_completion || return

    prev=${words[cword-1]}
    cur=${words[cword]}
    local subcmd=${words[1]}   # 'apply', 'logs', etc.

    # ----- Filename/path completion (only for filename contexts) -----
    # --filename is always a path flag (never 'follow'), so handle it regardless of subcommand.
    if [[ $prev == "--filename" ]]; then
        if declare -F _filedir >/dev/null 2>&1; then
            compopt -o filenames
            _filedir
            return
        fi
        compopt -o filenames
        compopt -o nospace
        local IFS=$'\n'; local candidates=($(compgen -f -- "$cur"))
        for i in "${!candidates[@]}"; do
            [[ -d "${candidates[i]}" && "${candidates[i]}" != */ ]] && candidates[i]="${candidates[i]}/"
        done
        COMPREPLY=( "${candidates[@]}" )
        return
    fi

    # Short -f can be either 'filename' OR 'follow' depending on subcommand.
    # Do NOT path-complete for 'logs -f' (follow).
    if [[ $prev == "-f" ]]; then
        case "$subcmd" in
            logs) ;;  # not a filename context; fall through to normal completion
            *)
                if declare -F _filedir >/dev/null 2>&1; then
                    compopt -o filenames
                    _filedir
                    return
                fi
                compopt -o filenames
                compopt -o nospace
                local IFS=$'\n'; local candidates=($(compgen -f -- "$cur"))
                for i in "${!candidates[@]}"; do
                    [[ -d "${candidates[i]}" && "${candidates[i]}" != */ ]] && candidates[i]="${candidates[i]}/"
                done
                COMPREPLY=( "${candidates[@]}" )
                return
                ;;
        esac
    fi
    # ---------------------------------------------------------------

    if (( cword == 1 )); then
        COMPREPLY=($(compgen -W "cn logs cc completion" -- "$cur"))
        return
    fi

    case ${words[1]} in
        cc)
            return
            ;;
        cn)
            COMPREPLY=($(compgen -W "$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}')" -- "$cur"))
            ;;
        logs)
            # complete pods for logs (and let kubectl handle containers with -c)
            COMPREPLY=($(compgen -W "$(kubectl get po -o jsonpath='{.items[*].metadata.name}')" -- "$cur"))
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"))
            ;;
        *)
            COMPREPLY=( $(kubectl __complete "${words[@]:1}" 2>/dev/null | grep -v '^:') )
            ;;
    esac
}
complete -F _km_completion km
`

const zshScript = `#compdef km
_km_completion() {
    local context state state_descr line
    _arguments -C \
        '1: :->command' \
        '*:: :->args'
    case $state in
        (command)
            local commands=(
                'cc:Switch context'
                'cn:Change namespace'
                'logs:View logs'
                'completion:Generate completion'
            )
            _describe 'command' commands
            ;;
        (args)
            case ${line[1]} in
                (cc) ;;
                (cn) _values 'namespaces' $(kubectl get namespaces -o name | sed 's/namespace\///') ;;
                (logs)
                    # For logs, -f is 'follow', not filename — complete pods, not paths.
                    _values 'pods' $(kubectl get pods -o name | sed 's/pod\///')
                    ;;
                (completion) _values 'shells' bash zsh fish ;;
                (*)
                    # If previous word was a filename flag, use path completion.
                    if [[ ${words[CURRENT-1]} == "--filename" ]]; then
                        _files
                    elif [[ ${words[CURRENT-1]} == "-f" ]]; then
                        # Only path-complete when subcommand is not 'logs'
                        if [[ ${line[1]} != "logs" ]]; then
                            _files
                            return
                        fi
                        # else fall through to kubectl completion below
                    fi
                    _values 'kubectl' $(kubectl __complete "${line[@]}" 2>/dev/null | grep -v '^:')
                    ;;
            esac
            ;;
    esac
}
compdef _km_completion km
`

const fishScript = `function __fish_km_complete
    set -l args (commandline -opc)
    set -l cur (commandline -ct)

    if test (count $args) -eq 1
        echo cc
        echo cn
        echo logs
        echo completion
        return
    end

    set -l prev ''
    if test (count $args) -ge 2
        set prev $args[-2]
    end
    set -l subcmd $args[2]

    # --filename is always path
    if test "$prev" = "--filename"
        __fish_complete_path $cur
        return
    end

    # Short -f: only path-complete when it is a filename flag (i.e., NOT for logs)
    if test "$prev" = "-f"
        switch $subcmd
            case logs
                # not a filename context; fall through to normal completion below
            case '*'
                __fish_complete_path $cur
                return
        end
    end

    switch $subcmd
        case cc
            return
        case cn
            kubectl get namespaces -o name | string replace 'namespace/' ''
        case logs
            kubectl get pods -o name | string replace 'pod/' ''
        case completion
            echo bash
            echo zsh
            echo fish
        case '*'
            kubectl __complete $args[2..-1] 2>/dev/null | grep -v '^:'
    end
end
complete -c km -f -a '(__fish_km_complete)'
`
