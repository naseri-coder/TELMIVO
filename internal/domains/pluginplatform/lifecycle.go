package pluginplatform

import sdk "github.com/naseri-coder/TELMIVO/internal/platform/plugincontract"

func Transition(from sdk.State, action sdk.Action) (sdk.State, error) {
	switch action {
	case sdk.Install:
		if from == sdk.Available {
			return sdk.Installed, nil
		}
	case sdk.Activate:
		if from == sdk.Installed || from == sdk.Disabled {
			return sdk.Active, nil
		}
	case sdk.Disable:
		if from == sdk.Active || from == sdk.Installed {
			return sdk.Disabled, nil
		}
	case sdk.RequestUninstall:
		if from == sdk.Disabled || from == sdk.Installed {
			return sdk.UninstallPending, nil
		}
	case sdk.CompleteUninstall:
		if from == sdk.UninstallPending {
			return sdk.Uninstalled, nil
		}
	}
	return from, sdk.ErrState
}

func activeSet(records []sdk.Installation) map[string]bool {
	active := map[string]bool{}
	for _, i := range records {
		if i.State == sdk.Active {
			active[i.Scope.PluginID] = true
		}
	}
	return active
}

func (r *Registry) validateRecords(records []sdk.Installation) error {
	for _, i := range records {
		if i.State != sdk.Active {
			continue
		}
		m, err := r.Manifest(i.Scope.PluginID)
		if err != nil {
			return err
		}
		if i.Version != m.Version || i.DataVersion != m.DataVersion {
			return sdk.ErrVersion
		}
		if err := m.Settings.ValidateConfiguration(i.Configuration); err != nil {
			return err
		}
		if err := sdk.ValidateGrant(m, sdk.Grant{Capabilities: i.Capabilities, LicenseScope: i.LicenseScope}); err != nil {
			return err
		}
	}
	return r.ValidateActive(activeSet(records))
}
