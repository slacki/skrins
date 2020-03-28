# Skrins

Uploads screenshots from given directory to remote location via sftp. 

Usage

```
go build
./skrins -p /path/to/screenshots -r remote.host:22 -ru remoteuser -pk /path/to/private/key -rp /path/on/remote/host -url https://url.pointing.to.your.screens/
```