package config

import "testing"

func TestAppDirRefusesTheRealDirectoryUnderTest(t *testing.T) {
	SetAppName("goTApaper")
	SetAppDir("")
	t.Setenv("GOTAPAPER_APP_DIR", "")

	defer func() {
		if recover() == nil {
			t.Fatal("AppDir resolved the real application directory under test")
		}
	}()
	AppDir()
}

func TestAppDirHonoursAnOverrideUnderTest(t *testing.T) {
	SetAppName("goTApaper")
	dir := t.TempDir()
	SetAppDir(dir)
	t.Cleanup(func() { SetAppDir("") })

	if got := AppDir(); got != dir {
		t.Fatalf("AppDir() = %q, want %q", got, dir)
	}
}
