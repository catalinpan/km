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
		os.Stdout.WriteString(bashScript)
	case "zsh":
		os.Stdout.WriteString(zshScript)
	case "fish":
		os.Stdout.WriteString(fishScript)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported shell type %q\n", shell)
		Exit(1)
	}
}

const bashScript = `# km flag/subcommand glossary — the only things this script "knows about".
# Anything not listed here flows straight through to kubectl.
#   --all           km logs flag (stream every pod)
#   -i / --interval km watch flag with value (refresh seconds)
#   --grep          km output filter, takes a value
#   cc cn whoami completion  km-only top-level subcommands
#   watch           km wrapper around kubectl with optional --interval
#   -l / --selector pod-only label completion (kubectl __complete doesn't
#                   enumerate labels; restricted to pods so a copy-pasted
#                   selector can't accidentally hit deployments/sts/etc.)

# Strip km-only args from "$@", echo the remainder one per line (preserving
# empty strings — important for the trailing word being completed).
__km_strip_own_args() {
    while (( $# > 0 )); do
        case "$1" in
            -i|--interval|--grep)
                if (( $# >= 2 )); then shift 2; else shift; fi
                ;;
            -i=*|--interval=*|--grep=*|--all)
                shift
                ;;
            *)
                printf '%s\n' "$1"
                shift
                ;;
        esac
    done
}

# Default bash COMP_WORDBREAKS contains '=', so "key=value" gets split into
# ("key" "=" "value"). kubectl __complete expects "key=value" as one token,
# so rejoin those triples before forwarding. Echoes one rejoined token per
# line (preserves empty trailing token).
__km_rejoin_eq() {
    local args=("$@")
    local out=()
    local i=0
    while (( i < ${#args[@]} )); do
        if [[ "${args[i]}" == "=" && ${#out[@]} -gt 0 ]]; then
            out[${#out[@]}-1]+="="
            if (( i+1 < ${#args[@]} )); then
                out[${#out[@]}-1]+="${args[i+1]}"
                (( i += 2 ))
                continue
            fi
        else
            out+=("${args[i]}")
        fi
        (( i++ ))
    done
    printf '%s\n' "${out[@]}"
}

# Forward to kubectl __complete with the given args. cobra prints
# "<candidate>\t<description>" lines plus a trailing ":N" directive line;
# strip both so the words land in COMPREPLY cleanly.
__km_kubectl_complete() {
    local cur=${COMP_WORDS[COMP_CWORD]}
    local candidates
    candidates=$(kubectl __complete "$@" 2>/dev/null | grep -v '^:' | cut -f1)
    COMPREPLY=( $(compgen -W "$candidates" -- "$cur") )
}

# True when the user is clearly working with pods: either the subcommand is
# 'logs' (implicitly pods) or one of pod/pods/po appears positionally.
__km_is_pods_context() {
    local subcmd=${1:-}
    shift || true
    [[ "$subcmd" == "logs" ]] && return 0
    local w
    for w in "$@"; do
        case "$w" in
            pod|pods|po) return 0 ;;
        esac
    done
    return 1
}

_km_completion() {
    local cur prev words cword
    _init_completion || return

    prev=${words[cword-1]}
    cur=${words[cword]}
    local subcmd=${words[1]:-}

    # File-path completion for filename flags. _filedir is a much better path
    # completer than kubectl __complete returns; this is a UX shortcut, not a
    # filter rule.
    if [[ $prev == "--filename" ]] || { [[ $prev == "-f" ]] && [[ $subcmd != "logs" ]]; }; then
        if declare -F _filedir >/dev/null 2>&1; then
            compopt -o filenames
            _filedir
            return
        fi
        compopt -o filenames
        compopt -o nospace
        local IFS=$'\n'
        COMPREPLY=( $(compgen -f -- "$cur") )
        return
    fi

    # Label completion for pods. Three trigger shapes because bash's default
    # COMP_WORDBREAKS contains '=', splitting "key=value" into three tokens:
    #   (a) prev is '-l'/'--selector' → typing the whole "key=val" or just
    #       "key". Return full "key=value" candidates.
    #   (b) prev is '=' and three tokens back is '-l'/'--selector' → typing
    #       the value side of "key=<cur>". Return value-only candidates.
    #   (c) cur is '=' and two tokens back is '-l'/'--selector' → cursor is
    #       parked on the '=' itself. Bash inserts AFTER the '=' wordbreak
    #       boundary, so we return value-only too (prefixing with '=' would
    #       produce a doubled '==').
    # Restricted to pods context so 'km delete deploy -l <TAB>' returns nothing.
    local _sel_value_mode=0
    local _sel_key=""
    if [[ $prev == "=" ]] && (( cword >= 3 )) \
       && { [[ ${words[cword-3]} == "-l" ]] || [[ ${words[cword-3]} == "--selector" ]]; }; then
        _sel_value_mode=1
        _sel_key="${words[cword-2]}"
    elif [[ $cur == "=" ]] && (( cword >= 2 )) \
         && { [[ ${words[cword-2]} == "-l" ]] || [[ ${words[cword-2]} == "--selector" ]]; }; then
        _sel_value_mode=1
        _sel_key="${words[cword-1]}"
        cur=""
    fi
    if [[ $prev == "-l" || $prev == "--selector" ]] || (( _sel_value_mode )); then
        if __km_is_pods_context "$subcmd" "${words[@]}"; then
            local ns_args=() i=0
            while (( i < ${#words[@]} )); do
                case "${words[i]}" in
                    -n|--namespace)
                        if (( i+1 < ${#words[@]} )); then
                            ns_args=(-n "${words[i+1]}")
                            break
                        fi
                        ;;
                    -n=*) ns_args=(-n "${words[i]#-n=}"); break ;;
                    --namespace=*) ns_args=(-n "${words[i]#--namespace=}"); break ;;
                esac
                (( i++ ))
            done
            local labels
            labels=$(kubectl get pods "${ns_args[@]}" \
                -o go-template='{{range .items}}{{range $k, $v := .metadata.labels}}{{$k}}={{$v}}{{"\n"}}{{end}}{{end}}' \
                2>/dev/null | sort -u)
            if (( _sel_value_mode )); then
                local vals=()
                while IFS= read -r line; do
                    [[ "$line" == "${_sel_key}="* ]] && vals+=("${line#${_sel_key}=}")
                done <<< "$labels"
                COMPREPLY=($(compgen -W "${vals[*]}" -- "$cur"))
            else
                COMPREPLY=($(compgen -W "$labels" -- "$cur"))
            fi
        fi
        return
    fi

    # First word: km terminal commands + kubectl's top-level commands.
    if (( cword == 1 )); then
        local km_cmds="cc cn watch whoami completion"
        local k_cmds
        k_cmds=$(kubectl __complete "" 2>/dev/null | grep -v '^:' | cut -f1)
        COMPREPLY=($(compgen -W "$km_cmds $k_cmds" -- "$cur"))
        return
    fi

    # km-only subcommands that have no kubectl equivalent.
    case "$subcmd" in
        cc|whoami)
            return
            ;;
        cn)
            local namespaces
            namespaces=$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)
            COMPREPLY=($(compgen -W "$namespaces" -- "$cur"))
            return
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"))
            return
            ;;
    esac

    # Everything else: strip km's own args, hand the rest to kubectl. For
    # 'watch', also drop the literal 'watch' keyword since km swallows it
    # before invoking kubectl.
    local start=1
    [[ "$subcmd" == "watch" ]] && start=2

    local raw=("${words[@]:$start}")
    local rejoined=()
    if (( ${#raw[@]} > 0 )); then
        while IFS= read -r line; do
            rejoined+=("$line")
        done < <(__km_rejoin_eq "${raw[@]}")
    fi
    local cleaned=()
    if (( ${#rejoined[@]} > 0 )); then
        while IFS= read -r line; do
            cleaned+=("$line")
        done < <(__km_strip_own_args "${rejoined[@]}")
    fi
    if (( ${#cleaned[@]} == 0 )); then
        cleaned=("")
    fi

    __km_kubectl_complete "${cleaned[@]}"
}
complete -F _km_completion km
`

const zshScript = `#compdef km
# km flag/subcommand glossary — see bash script for the rationale. The only
# km-specific things this script knows about are:
#   --all           (km logs)
#   -i / --interval (km watch, takes a value)
#   cc cn whoami completion  km-only top-level subcommands
#   watch           km wrapper subcommand
#   -l / --selector pod-only label completion (kubectl __complete does not
#                   enumerate labels; restricted to pods so a copy-pasted
#                   selector can't accidentally hit deployments/sts/etc.)
# Everything else is forwarded verbatim to kubectl __complete.

# Strip km-only args from "$@", echo remainder one per line.
__km_strip_own_args() {
    while (( $# > 0 )); do
        case "$1" in
            -i|--interval|--grep)
                if (( $# >= 2 )); then shift 2; else shift; fi
                ;;
            -i=*|--interval=*|--grep=*|--all)
                shift
                ;;
            *)
                print -r -- "$1"
                shift
                ;;
        esac
    done
}

# Forward to kubectl __complete; strip descriptions and the directive line.
__km_kubectl_complete() {
    local -a candidates
    candidates=("${(@f)$(kubectl __complete "$@" 2>/dev/null | grep -v '^:' | cut -f1)}")
    compadd -a candidates
}

_km_completion() {
    local context state state_descr line
    _arguments -C \
        '1: :->command' \
        '*:: :->args'
    case $state in
        (command)
            local -a km_commands
            km_commands=(
                'cc:Switch context'
                'cn:Change namespace'
                'watch:Watch a kubectl command'
                'whoami:Show current session info'
                'completion:Generate completion'
            )
            _describe 'km command' km_commands
            local -a k_commands
            k_commands=("${(@f)$(kubectl __complete '' 2>/dev/null | grep -v '^:' | cut -f1)}")
            compadd -a k_commands
            ;;
        (args)
            # File-path completion shortcut for filename flags.
            if [[ ${words[CURRENT-1]} == "--filename" ]] || { [[ ${words[CURRENT-1]} == "-f" ]] && [[ ${line[1]} != "logs" ]] }; then
                _files
                return
            fi

            # Label completion for pods. Triggers only when the line is clearly
            # about pods (subcmd 'logs', or pod/pods/po appears positionally).
            if [[ ${words[CURRENT-1]} == "-l" || ${words[CURRENT-1]} == "--selector" ]]; then
                local is_pods=0 w
                if [[ ${line[1]} == "logs" ]]; then
                    is_pods=1
                else
                    for w in "${line[@]}"; do
                        case "$w" in
                            pod|pods|po) is_pods=1; break ;;
                        esac
                    done
                fi
                if (( is_pods )); then
                    local -a ns_args
                    local i=1
                    while (( i <= ${#line} )); do
                        case "${line[i]}" in
                            -n|--namespace)
                                if (( i+1 <= ${#line} )); then
                                    ns_args=(-n "${line[i+1]}")
                                    break
                                fi
                                ;;
                            -n=*) ns_args=(-n "${line[i]#-n=}"); break ;;
                            --namespace=*) ns_args=(-n "${line[i]#--namespace=}"); break ;;
                        esac
                        (( i++ ))
                    done
                    local -a labels
                    labels=("${(@f)$(kubectl get pods "${ns_args[@]}" -o go-template='{{range .items}}{{range $k, $v := .metadata.labels}}{{$k}}={{$v}}{{"\n"}}{{end}}{{end}}' 2>/dev/null | sort -u)}")
                    compadd -a labels
                fi
                return
            fi

            case ${line[1]} in
                (cc|whoami) ;;
                (cn) _values 'namespaces' $(kubectl get namespaces -o name | sed 's|namespace/||') ;;
                (completion) _values 'shells' bash zsh fish ;;
                (*)
                    # Strip km-only args (and the literal 'watch' if present),
                    # then forward to kubectl __complete.
                    local -a raw
                    if [[ ${line[1]} == "watch" ]]; then
                        raw=(${line[@]:1})
                    else
                        raw=(${line[@]})
                    fi
                    local -a cleaned
                    cleaned=("${(@f)$(__km_strip_own_args "${raw[@]}")}")
                    if (( ${#cleaned} == 0 )); then
                        cleaned=('')
                    fi
                    __km_kubectl_complete "${cleaned[@]}"
                    ;;
            esac
            ;;
    esac
}
compdef _km_completion km
`

const fishScript = `# km flag/subcommand glossary — see bash script for the rationale. The only
# km-specific things this script knows about are:
#   --all           (km logs)
#   -i / --interval (km watch, takes a value)
#   cc cn whoami completion  km-only top-level subcommands
#   watch           km wrapper subcommand
#   -l / --selector pod-only label completion (kubectl __complete does not
#                   enumerate labels; restricted to pods so a copy-pasted
#                   selector can't accidentally hit deployments/sts/etc.)
# Everything else is forwarded verbatim to kubectl __complete.

# Strip km-only args from $argv; print remainder one per line (preserves empty
# strings — important for the trailing word being completed).
function __km_strip_own_args
    set -l i 1
    while test $i -le (count $argv)
        switch $argv[$i]
            case -i --interval --grep
                set i (math $i + 2)
            case '-i=*' '--interval=*' '--grep=*' --all
                set i (math $i + 1)
            case '*'
                printf '%s\n' $argv[$i]
                set i (math $i + 1)
        end
    end
end

# Forward to kubectl __complete; fish handles the tab-description format
# natively, so we only strip the trailing ":N" directive line.
function __km_kubectl_complete
    kubectl __complete $argv 2>/dev/null | grep -v '^:'
end

function __fish_km_complete
    set -l args (commandline -opc)
    set -l cur (commandline -ct)

    if test (count $args) -eq 1
        echo cc
        echo cn
        echo watch
        echo whoami
        echo completion
        __km_kubectl_complete ''
        return
    end

    set -l prev ''
    if test (count $args) -ge 2
        set prev $args[-2]
    end
    set -l subcmd $args[2]

    # File-path completion shortcut for filename flags.
    if test "$prev" = "--filename"
        __fish_complete_path $cur
        return
    end
    if test "$prev" = "-f"; and test "$subcmd" != "logs"
        __fish_complete_path $cur
        return
    end

    # Label completion for pods. Triggers only when the line is clearly
    # about pods (subcmd 'logs', or pod/pods/po appears positionally).
    if test "$prev" = "-l"; or test "$prev" = "--selector"
        set -l is_pods 0
        if test "$subcmd" = "logs"
            set is_pods 1
        else
            for w in $args
                if test "$w" = "pod"; or test "$w" = "pods"; or test "$w" = "po"
                    set is_pods 1
                    break
                end
            end
        end
        if test $is_pods -eq 1
            set -l ns
            set -l i 1
            while test $i -le (count $args)
                switch $args[$i]
                    case -n --namespace
                        if test (math $i + 1) -le (count $args)
                            set ns $args[(math $i + 1)]
                            break
                        end
                    case '-n=*'
                        set ns (string replace -- '-n=' '' $args[$i])
                        break
                    case '--namespace=*'
                        set ns (string replace -- '--namespace=' '' $args[$i])
                        break
                end
                set i (math $i + 1)
            end
            set -l ns_args
            if test -n "$ns"
                set ns_args -n $ns
            end
            kubectl get pods $ns_args -o go-template='{{range .items}}{{range $k, $v := .metadata.labels}}{{$k}}={{$v}}{{"\n"}}{{end}}{{end}}' 2>/dev/null | sort -u
        end
        return
    end

    switch $subcmd
        case cc whoami
            return
        case cn
            kubectl get namespaces -o name | string replace 'namespace/' ''
            return
        case completion
            echo bash
            echo zsh
            echo fish
            return
    end

    # Strip km-only args (and the literal 'watch' if present), then forward to
    # kubectl __complete.
    set -l raw
    if test "$subcmd" = "watch"
        set raw $args[3..-1]
    else
        set raw $args[2..-1]
    end
    set -l cleaned (__km_strip_own_args $raw)
    if test (count $cleaned) -eq 0
        set cleaned ''
    end
    __km_kubectl_complete $cleaned
end
complete -c km -f -a '(__fish_km_complete)'
`
