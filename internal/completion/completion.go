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
    if (( $cword == 1 )); then
        COMPREPLY=($(compgen -W "cn logs cc completion" -- "$cur"))
        return
    fi
    case ${words[1]} in
        cc) return ;;
        cn)
            COMPREPLY=($(compgen -W "$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}')" -- "$cur")) ;;
        logs)
            COMPREPLY=($(compgen -W "$(kubectl get po -o jsonpath='{.items[*].metadata.name}')" -- "$cur")) ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")) ;;
        *)
            COMPREPLY=($(kubectl __complete "${words[@]:1}" 2>/dev/null | grep -v '^:')) ;;
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
            _describe 'command' commands ;;
        (args)
            case ${line[1]} in
                (cc) ;;
                (cn) _values 'namespaces' $(kubectl get namespaces -o name | sed 's/namespace\///') ;;
                (logs) _values 'pods' $(kubectl get pods -o name | sed 's/pod\///') ;;
                (completion) _values 'shells' bash zsh fish ;;
                (*) _values 'kubectl' $(kubectl __complete "${line[@]}" 2>/dev/null | grep -v '^:') ;;
            esac ;;
    esac
}
compdef _km_completion km
`

const fishScript = `function __fish_km_complete
    set -l args (commandline -opc)
    if test (count $args) -eq 1
        echo cc\ncn\nlogs\ncompletion
        return
    end
    switch $args[2]
        case cc
            return
        case cn
            kubectl get namespaces -o name | string replace 'namespace/' ''
        case logs
            kubectl get pods -o name | string replace 'pod/' ''
        case completion
            echo bash\nzsh\nfish
        case '*'
            kubectl __complete $args[2..-1] 2>/dev/null | grep -v '^:'
    end
end
complete -c km -f -a '(__fish_km_complete)'
`
