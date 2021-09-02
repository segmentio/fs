# fs
Go package exposing minimal APIs to watch unix file systems.

## Motivation

Go has long had a package to receive notification from file system
changes: [fsnotify](https://pkg.go.dev/github.com/fsnotify/fsnotify).
The package took on the challenging task to provide cross-platform solution,
and inherits complexity and limitations. One of these limitations is the
inability to watch for changes on symbolic links, which adds barriers to using
`fsnotify` in contexts like kubernetes config maps (which are exposed to pods
via symbolic links).

The `fs` package aims to address these limitations by giving up some of the
cross-platform goals and providing a simpler API mirrored after the standard
`signal` package.

## Usage Example

A key difference of the `fs` package is it installs _one-shot_ notifications
instead of persistent watchers. This design decision was made after observing
that programs tend to make adjustments to the watchers after each event,
especially when files are created, removed, or renamed, the persistence ends
up forcing complexity on the program.

```go
import (
    "github.com/segmentio/fs"
)

...

for {
    ch := make(chan string)
    if err := fs.Notify(ch, "/dir", "/dir/file"); err != nil {
        return err
    }

    select {
    case path := <-ch:
        switch path {
        case "/dir":
            ...
        case "/dir/file":
            ...
        }

    case <-ctx.Done():
        fs.Stop(ch)
        return ctx.Err()
    }
}
```
