package setter

import (
	"testing"

	"github.com/genzj/goTApaper/config"
)

// TestRegisterContributesEnumValue pins the property that the `setter`
// setting's accepted values track the registry.
//
// Registration happens from init functions across several files, which Go runs
// in filename order, so a setter defined in a file sorting after setter.go
// registers after this package's own init. Anything that snapshotted the
// registry up front would silently omit it, and the setting would then reject
// a setter that is in fact available.
func TestRegisterContributesEnumValue(t *testing.T) {
	const late = "zz-test-setter"

	Setters.Register(late, nil)
	t.Cleanup(func() { delete(Setters.RegistryMap, late) })

	def, ok := config.Lookup(config.KeyWallpaperSetter)
	if !ok {
		t.Fatal("the setter setting has no definition")
	}

	for _, v := range def.EnumValues {
		if v == late {
			return
		}
	}
	t.Errorf("a setter registered after init is missing from %v", def.EnumValues)
}

// TestPlatformSetterIsRegistered guards against the enum being empty, which
// would make every value invalid and so block any refresh.
func TestPlatformSetterIsRegistered(t *testing.T) {
	def, ok := config.Lookup(config.KeyWallpaperSetter)
	if !ok {
		t.Fatal("the setter setting has no definition")
	}
	if len(def.EnumValues) == 0 {
		t.Fatal("no setters registered for this platform")
	}

	// The default must itself be acceptable, or a fresh install cannot refresh.
	if err := def.ValidateValue(config.DefaultSetter); err != nil {
		t.Errorf("the default setter %q is not among %v", config.DefaultSetter, def.EnumValues)
	}
}
