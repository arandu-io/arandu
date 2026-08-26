package config

import "fmt"

// Disk is where uploaded files live.
type Disk string

// The supported disks. Object storage first, because that is what a deployed
// application uses; local exists for development and for a single machine.
const (
	// DiskLocal writes under storage/app. One machine only: a second replica
	// does not see what the first one wrote.
	DiskLocal Disk = "local"
	// DiskR2 is Cloudflare R2, the suggested default -- S3 protocol, no egress
	// fee.
	DiskR2 Disk = "r2"
	// DiskS3 is Amazon S3 and anything else that speaks its API.
	DiskS3 Disk = "s3"
)

// Filesystems is where uploaded files live, and what it takes to reach them.
//
// There is no public disk and no symlink into one. Assets are embedded in the
// binary and an uploaded file is served by a handler that checked a Grant first
// -- a directory the web server publishes is a directory with no policy in front
// of it.
type Filesystems struct {
	// Default is the disk a write goes to when none is named.
	Default Disk

	// Root is where DiskLocal writes.
	Root string

	// Bucket, Endpoint and Region describe the object store.
	Bucket   string
	Endpoint string
	Region   string

	// AccessKeyID and SecretAccessKey authenticate against it. They come from
	// the environment and are never written to a file in this repository.
	AccessKeyID     string
	SecretAccessKey string
}

func loadFilesystems() (Filesystems, error) {
	disk := Disk(env("FILESYSTEM_DISK", string(DiskLocal)))
	switch disk {
	case DiskLocal, DiskR2, DiskS3:
	default:
		return Filesystems{}, fmt.Errorf("FILESYSTEM_DISK has unsupported value %q; expected local, r2 or s3", disk)
	}
	return Filesystems{
		Default:         disk,
		Root:            env("FILESYSTEM_ROOT", "storage/app/private"),
		Bucket:          env("FILESYSTEM_BUCKET", ""),
		Endpoint:        env("FILESYSTEM_ENDPOINT", ""),
		Region:          env("FILESYSTEM_REGION", "auto"),
		AccessKeyID:     env("FILESYSTEM_ACCESS_KEY_ID", ""),
		SecretAccessKey: env("FILESYSTEM_SECRET_ACCESS_KEY", ""),
	}, nil
}
