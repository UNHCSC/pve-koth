package koth

import (
	"fmt"
	"strings"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
)

const CompetitionSchemaVersion = 2

func NormalizeGuestConfiguration(request *db.CreateCompetitionRequest) error {
	if request == nil {
		return fmt.Errorf("competition request is nil")
	}
	if request.GuestConfigNormalized {
		return nil
	}
	if err := applyConfiguredVMTemplates(request); err != nil {
		return err
	}

	var err error
	switch request.SchemaVersion {
	case 0, 1:
		if len(request.GuestSpecTemplates) > 0 || len(request.TeamGuestConfigs) > 0 {
			return fmt.Errorf("guestSpecTemplates and teamGuestConfigs require schemaVersion %d", CompetitionSchemaVersion)
		}
		err = normalizeLegacyGuestConfiguration(request)
	case CompetitionSchemaVersion:
		if len(request.ContainerSpecsTemplates) > 0 || len(request.TeamContainerConfigs) > 0 {
			return fmt.Errorf("schemaVersion %d cannot mix guest and legacy container configuration", CompetitionSchemaVersion)
		}
		err = normalizeVersionTwoGuestConfiguration(request)
	default:
		return fmt.Errorf("unsupported schemaVersion %d (latest supported version is %d)", request.SchemaVersion, CompetitionSchemaVersion)
	}
	if err != nil {
		return err
	}
	request.GuestConfigNormalized = true
	return nil
}

func applyConfiguredVMTemplates(request *db.CreateCompetitionRequest) error {
	for name, template := range request.GuestSpecTemplates {
		template.TemplateRef = strings.TrimSpace(template.TemplateRef)
		if template.TemplateRef == "" {
			continue
		}

		allowed, ok := config.Config.VMRestrictions.Template(template.TemplateRef)
		if !ok {
			return fmt.Errorf("guest template %q references VM template %q which is not allowed", name, template.TemplateRef)
		}
		providedKind := db.GuestKind(strings.ToLower(strings.TrimSpace(string(template.Kind))))
		providedOS := db.GuestOS(strings.ToLower(strings.TrimSpace(string(template.OS))))
		providedShell := db.ScriptShell(strings.ToLower(strings.TrimSpace(string(template.Shell))))
		providedConfigurator := db.NetworkConfigurator(strings.ToLower(strings.TrimSpace(string(template.NetworkConfigurator))))
		if providedKind != "" && providedKind != db.GuestKindQEMU {
			return fmt.Errorf("guest template %q: templateRef requires kind %q", name, db.GuestKindQEMU)
		}
		if template.TemplateVMID != 0 && template.TemplateVMID != allowed.VMID {
			return fmt.Errorf("guest template %q: templateVMID conflicts with configured VM template %q", name, template.TemplateRef)
		}
		if providedOS != "" && providedOS != db.GuestOS(allowed.OS) {
			return fmt.Errorf("guest template %q: os conflicts with configured VM template %q", name, template.TemplateRef)
		}
		if providedShell != "" && providedShell != db.ScriptShell(allowed.Shell) {
			return fmt.Errorf("guest template %q: shell conflicts with configured VM template %q", name, template.TemplateRef)
		}
		if providedConfigurator != "" && providedConfigurator != db.NetworkConfigurator(allowed.NetworkConfigurator) {
			return fmt.Errorf("guest template %q: networkConfigurator conflicts with configured VM template %q", name, template.TemplateRef)
		}
		if template.BootDisk != "" && strings.ToLower(strings.TrimSpace(template.BootDisk)) != allowed.BootDisk {
			return fmt.Errorf("guest template %q: bootDisk conflicts with configured VM template %q", name, template.TemplateRef)
		}

		template.Kind = db.GuestKindQEMU
		template.TemplateVMID = allowed.VMID
		template.OS = db.GuestOS(allowed.OS)
		template.Shell = db.ScriptShell(allowed.Shell)
		template.NetworkConfigurator = db.NetworkConfigurator(allowed.NetworkConfigurator)
		template.BootDisk = allowed.BootDisk
		request.GuestSpecTemplates[name] = template
	}
	return nil
}

func normalizeLegacyGuestConfiguration(request *db.CreateCompetitionRequest) error {
	legacy, err := BuildContainerSpecTemplateIndex(request.ContainerSpecsTemplates)
	if err != nil {
		return err
	}

	guestTemplates := make(map[string]db.GuestSpecTemplate, len(legacy))
	for name, template := range legacy {
		guestTemplates[name] = db.GuestSpecTemplate{
			Kind:                db.GuestKindLXC,
			OS:                  db.GuestOSLinux,
			Shell:               db.ScriptShellBash,
			TemplatePath:        strings.TrimSpace(template.TemplatePath),
			StoragePool:         strings.TrimSpace(template.StoragePool),
			Username:            "root",
			Password:            template.RootPassword,
			DiskSizeGB:          template.StorageSizeGB,
			MemoryMB:            template.MemoryMB,
			Cores:               template.Cores,
			NetworkConfigurator: db.NetworkConfiguratorPVE,
		}
	}

	guestConfigs := make([]db.TeamGuestConfig, 0, len(request.TeamContainerConfigs))
	for _, config := range request.TeamContainerConfigs {
		guestConfigs = append(guestConfigs, db.TeamGuestConfig{
			Name:               config.Name,
			LastOctetValue:     config.LastOctetValue,
			SetupScript:        append([]string(nil), config.SetupScript...),
			ScoringScript:      append([]string(nil), config.ScoringScript...),
			ScoringSchema:      append([]db.ScoringCheck(nil), config.ScoringSchema...),
			GuestSpecsTemplate: config.ContainerSpecsTemplate,
		})
	}

	request.GuestSpecTemplates = guestTemplates
	request.TeamGuestConfigs = guestConfigs
	request.GuestTemplateLookup = guestTemplates
	request.TemplateLookup = legacy
	return validateTeamGuestReferences(guestTemplates, guestConfigs)
}

func normalizeVersionTwoGuestConfiguration(request *db.CreateCompetitionRequest) error {
	lookup, err := BuildGuestSpecTemplateIndex(request.GuestSpecTemplates)
	if err != nil {
		return err
	}
	if err = validateTeamGuestReferences(lookup, request.TeamGuestConfigs); err != nil {
		return err
	}

	request.GuestSpecTemplates = lookup
	request.GuestTemplateLookup = lookup

	allLXC := true
	legacyTemplates := make(map[string]db.ContainerSpecTemplate, len(lookup))
	for name, template := range lookup {
		if template.Kind != db.GuestKindLXC {
			allLXC = false
			continue
		}
		legacyTemplates[name] = db.ContainerSpecTemplate{
			TemplatePath:  template.TemplatePath,
			StoragePool:   template.StoragePool,
			RootPassword:  template.Password,
			StorageSizeGB: template.DiskSizeGB,
			MemoryMB:      template.MemoryMB,
			Cores:         template.Cores,
		}
	}

	if !allLXC {
		return nil
	}

	legacyConfigs := make([]db.TeamContainerConfig, 0, len(request.TeamGuestConfigs))
	for _, config := range request.TeamGuestConfigs {
		legacyConfigs = append(legacyConfigs, db.TeamContainerConfig{
			Name:                   config.Name,
			LastOctetValue:         config.LastOctetValue,
			SetupScript:            append([]string(nil), config.SetupScript...),
			ScoringScript:          append([]string(nil), config.ScoringScript...),
			ScoringSchema:          append([]db.ScoringCheck(nil), config.ScoringSchema...),
			ContainerSpecsTemplate: config.GuestSpecsTemplate,
		})
	}

	request.ContainerSpecsTemplates = legacyTemplates
	request.TeamContainerConfigs = legacyConfigs
	request.TemplateLookup = legacyTemplates
	return nil
}

func BuildGuestSpecTemplateIndex(raw map[string]db.GuestSpecTemplate) (map[string]db.GuestSpecTemplate, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("guestSpecTemplates must include at least one entry")
	}

	index := make(map[string]db.GuestSpecTemplate, len(raw))
	for rawName, rawTemplate := range raw {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, fmt.Errorf("guestSpecTemplates contains an empty name")
		}
		if _, exists := index[name]; exists {
			return nil, fmt.Errorf("duplicate guest template name %q", name)
		}

		template, err := normalizeGuestSpecTemplate(name, rawTemplate)
		if err != nil {
			return nil, err
		}
		index[name] = template
	}

	return index, nil
}

func ResolveGuestSpecTemplate(index map[string]db.GuestSpecTemplate, rawName string) (db.GuestSpecTemplate, error) {
	if len(index) == 0 {
		return db.GuestSpecTemplate{}, fmt.Errorf("guestSpecTemplates is not defined")
	}

	name := strings.TrimSpace(rawName)
	if name == "" {
		return db.GuestSpecTemplate{}, fmt.Errorf("guest config missing guestSpecsTemplate reference")
	}

	template, ok := index[name]
	if !ok {
		return db.GuestSpecTemplate{}, fmt.Errorf("template %q is not defined", name)
	}
	return template, nil
}

func normalizeGuestSpecTemplate(name string, template db.GuestSpecTemplate) (db.GuestSpecTemplate, error) {
	template.Kind = db.GuestKind(strings.ToLower(strings.TrimSpace(string(template.Kind))))
	template.OS = db.GuestOS(strings.ToLower(strings.TrimSpace(string(template.OS))))
	template.Shell = db.ScriptShell(strings.ToLower(strings.TrimSpace(string(template.Shell))))
	template.NetworkConfigurator = db.NetworkConfigurator(strings.ToLower(strings.TrimSpace(string(template.NetworkConfigurator))))
	template.TemplateRef = strings.TrimSpace(template.TemplateRef)
	template.TemplatePath = strings.TrimSpace(template.TemplatePath)
	template.StoragePool = strings.TrimSpace(template.StoragePool)
	template.Username = strings.TrimSpace(template.Username)
	template.BootDisk = strings.TrimSpace(template.BootDisk)

	if template.StoragePool == "" {
		return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing storagePool", name)
	}
	if template.DiskSizeGB <= 0 {
		return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q invalid diskSizeGB (%d)", name, template.DiskSizeGB)
	}
	if template.MemoryMB <= 0 {
		return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q invalid memoryMB (%d)", name, template.MemoryMB)
	}
	if template.Cores <= 0 {
		return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q invalid cores (%d)", name, template.Cores)
	}

	switch template.Kind {
	case db.GuestKindLXC:
		if template.TemplateRef != "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: LXC guests cannot set templateRef", name)
		}
		if template.OS == "" {
			template.OS = db.GuestOSLinux
		}
		if template.OS != db.GuestOSLinux {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: LXC guests must use os %q", name, db.GuestOSLinux)
		}
		if template.Shell == "" {
			template.Shell = db.ScriptShellBash
		}
		if template.Shell != db.ScriptShellBash {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: LXC guests must use shell %q", name, db.ScriptShellBash)
		}
		if template.NetworkConfigurator == "" {
			template.NetworkConfigurator = db.NetworkConfiguratorPVE
		}
		if template.NetworkConfigurator != db.NetworkConfiguratorPVE {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: LXC guests must use networkConfigurator %q", name, db.NetworkConfiguratorPVE)
		}
		if template.TemplatePath == "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing templatePath", name)
		}
		if template.TemplateVMID != 0 {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: LXC guests cannot set templateVMID", name)
		}
		if template.Username == "" {
			template.Username = "root"
		}
		if template.Password == "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing password", name)
		}
	case db.GuestKindQEMU:
		if template.TemplateRef == "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing templateRef", name)
		}
		if template.TemplateVMID <= 0 {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing templateVMID", name)
		}
		if template.TemplatePath != "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: QEMU guests cannot set templatePath", name)
		}
		if template.OS != db.GuestOSLinux && template.OS != db.GuestOSWindows {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q has invalid os %q", name, template.OS)
		}
		if template.Shell == "" {
			if template.OS == db.GuestOSWindows {
				template.Shell = db.ScriptShellPowerShell
			} else {
				template.Shell = db.ScriptShellBash
			}
		}
		if template.OS == db.GuestOSLinux && template.Shell != db.ScriptShellBash {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: Linux guests must use shell %q", name, db.ScriptShellBash)
		}
		if template.OS == db.GuestOSWindows && template.Shell != db.ScriptShellPowerShell {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q: Windows guests must use shell %q", name, db.ScriptShellPowerShell)
		}
		if template.NetworkConfigurator == "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing networkConfigurator", name)
		}
		if !validQEMUNetworkConfigurator(template.OS, template.NetworkConfigurator) {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q has incompatible networkConfigurator %q for os %q", name, template.NetworkConfigurator, template.OS)
		}
		if template.Username == "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing username", name)
		}
		if template.Password == "" {
			return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q missing password", name)
		}
		if template.BootDisk == "" {
			template.BootDisk = "scsi0"
		}
	default:
		return db.GuestSpecTemplate{}, fmt.Errorf("guest template %q has invalid kind %q", name, template.Kind)
	}

	return template, nil
}

func validQEMUNetworkConfigurator(osType db.GuestOS, configurator db.NetworkConfigurator) bool {
	if osType == db.GuestOSWindows {
		return configurator == db.NetworkConfiguratorPowerShell
	}
	return configurator == db.NetworkConfiguratorNetworkManager || configurator == db.NetworkConfiguratorNetplan || configurator == db.NetworkConfiguratorSystemdNetworkd
}

func validateTeamGuestReferences(templates map[string]db.GuestSpecTemplate, configs []db.TeamGuestConfig) error {
	for index, config := range configs {
		name := strings.TrimSpace(config.Name)
		if name == "" {
			return fmt.Errorf("teamGuestConfigs[%d] missing name", index)
		}
		if _, err := ResolveGuestSpecTemplate(templates, config.GuestSpecsTemplate); err != nil {
			return fmt.Errorf("team guest %s references invalid template %q: %w", name, config.GuestSpecsTemplate, err)
		}
	}
	return nil
}
