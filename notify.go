package fs

// Notify installs the events channel as a receiver for file system change
// notifications that occur on the paths passed as arguments.
//
// Calling Notify with an events channel that was already installed replaces
// the previous installation. Calling Notify with no paths uninstalls the
// events channel in a way similar to calling Stop.
//
// Notifications are only triggered once, if the program needs to receive
// further notification for the paths, it needs to re-install the events
// channel by calling Notify again.
//
// When a change occurs on one of the paths, the events channel receives the
// path that was modified. Paths may point to directories or files. If a path
// is removed the OS removes the watchers installed by Notify (after emitting
// a notification).
func Notify(events chan<- string, paths ...string) error {
	if len(paths) == 0 {
		stop(events)
		return nil
	} else {
		return notify(events, paths)
	}
}

// Stop uninstalls an events channel previously installed by a call to Notify.
//
// Calling stops is not necessary if the events channel was triggered because
// it will be automatically uninstalled.
func Stop(events chan<- string) {
	stop(events)
}
