package btrfs

// Fakes is a bag of optional replacements for the btrfs package's external
// operations. Only the non-nil fields are swapped in by WithFakes; the rest
// keep their real implementations.
type Fakes struct {
	Snapshot          func(src, dst string) error
	Delete            func(path string) error
	SubvolumeExists   func(path string) bool
	OpenTopVol        func(mountpoint, target string) (*TopVol, error)
	CloseTopVol       func(tv *TopVol) error
	FindmntSource     func(mountpoint string) (string, error)
	FindmntUUID       func(mountpoint string) (string, error)
	KernelVersion     func() (string, error)
	IsInsideSnapshot  func() (bool, error)
	CurrentRootSubvol func() (string, error)
}

// WithFakes installs the non-nil fields of f as the active implementations and
// returns a restore function. Idiom:
//
//	defer btrfs.WithFakes(btrfs.Fakes{Snapshot: func(...) error { ... }})()
//
// Not safe for use by concurrent tests on the same package.
func WithFakes(f Fakes) func() {
	prev := Fakes{
		Snapshot:          snapshotFn,
		Delete:            deleteFn,
		SubvolumeExists:   subvolumeExistsFn,
		OpenTopVol:        openTopVolFn,
		CloseTopVol:       closeTopVolFn,
		FindmntSource:     findmntSourceFn,
		FindmntUUID:       findmntUUIDFn,
		KernelVersion:     kernelVersionFn,
		IsInsideSnapshot:  isInsideSnapshotFn,
		CurrentRootSubvol: currentRootSubvolFn,
	}
	if f.Snapshot != nil {
		snapshotFn = f.Snapshot
	}
	if f.Delete != nil {
		deleteFn = f.Delete
	}
	if f.SubvolumeExists != nil {
		subvolumeExistsFn = f.SubvolumeExists
	}
	if f.OpenTopVol != nil {
		openTopVolFn = f.OpenTopVol
	}
	if f.CloseTopVol != nil {
		closeTopVolFn = f.CloseTopVol
	}
	if f.FindmntSource != nil {
		findmntSourceFn = f.FindmntSource
	}
	if f.FindmntUUID != nil {
		findmntUUIDFn = f.FindmntUUID
	}
	if f.KernelVersion != nil {
		kernelVersionFn = f.KernelVersion
	}
	if f.IsInsideSnapshot != nil {
		isInsideSnapshotFn = f.IsInsideSnapshot
	}
	if f.CurrentRootSubvol != nil {
		currentRootSubvolFn = f.CurrentRootSubvol
	}
	return func() {
		snapshotFn = prev.Snapshot
		deleteFn = prev.Delete
		subvolumeExistsFn = prev.SubvolumeExists
		openTopVolFn = prev.OpenTopVol
		closeTopVolFn = prev.CloseTopVol
		findmntSourceFn = prev.FindmntSource
		findmntUUIDFn = prev.FindmntUUID
		kernelVersionFn = prev.KernelVersion
		isInsideSnapshotFn = prev.IsInsideSnapshot
		currentRootSubvolFn = prev.CurrentRootSubvol
	}
}
