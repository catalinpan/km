# km

`km` is a thin wrapper for kubectl to enhance few features.
There are some short commands to set the cluster/namespace context, completion install and helper for logs.
Any other command will be proxied to kubectl so km command can be used in the same way as kubectl.

Check `km -h` for all the available options.

## Demo
![Demo](examples/demo.gif)

## Features

- context change for the cluster and the namespace with short commands
- bash and zsh completion (kubectl completion is required), use `km -h` for install options
- single binary, no dependencies required

## Install

### Linux
```
sudo curl -L \
$(curl -s https://api.github.com/repos/catalinpan/km/releases/latest \
| jq -r '.assets[] | select(.name == "km-linux-amd64") | .browser_download_url') \
-o /usr/local/bin/km && sudo chmod +x /usr/local/bin/km
```

### Change cluster context with fzf 
```
km cc 
```

### Change namespace context with fzf select or completion and without
```
km cn
km cn default
```

### Get pod logs with fzf select or completion
```
km logs
```

# Commands

## Run main.go app
```
go run ./cmd/km/main.go get po
 ```

## Run tests
```
go test -v ./...
```
## Build
```
go build -o km ./cmd/km/main.go
```