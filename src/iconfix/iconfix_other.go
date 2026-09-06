//go:build !windows

package iconfix

func CreateLaunchShortcut(lnkPath, javaBinary, args, workDir, iconPath string) error {
	return nil
}
