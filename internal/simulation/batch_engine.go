package simulation

import (
	"fmt"
	"os"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
)

type batchEnergyPlusResolution struct {
	ExecutablePath string
	FailureStatus  string
	Error          string
}

func resolveBatchEnergyPlusExecutable(inputPath string, explicitPath string, installations []EnergyPlusInstallSetting) batchEnergyPlusResolution {
	if executablePath := strings.TrimSpace(explicitPath); executablePath != "" {
		return batchEnergyPlusResolution{ExecutablePath: executablePath}
	}
	if len(installations) == 0 {
		return batchEnergyPlusResolution{
			FailureStatus: "missing_energyplus",
			Error:         "No EnergyPlus installation is configured.",
		}
	}

	content, err := os.ReadFile(inputPath)
	if err != nil {
		return batchEnergyPlusResolution{
			FailureStatus: "failed",
			Error:         fmt.Sprintf("EnergyPlus input version could not be read: %v", err),
		}
	}
	model, err := epinput.Parse(inputPath, content)
	if err != nil {
		return batchEnergyPlusResolution{
			FailureStatus: "failed",
			Error:         fmt.Sprintf("EnergyPlus input version could not be parsed: %v", err),
		}
	}

	requiredVersion := strings.TrimSpace(model.Version.Raw)
	requiredMajor, requiredMinor, versionKnown := energyPlusMajorMinor(requiredVersion)
	if !versionKnown {
		return batchEnergyPlusResolution{ExecutablePath: installations[0].ExecutablePath}
	}
	for _, installation := range installations {
		major, minor, known := energyPlusMajorMinor(installation.Version)
		if known && major == requiredMajor && minor == requiredMinor {
			return batchEnergyPlusResolution{ExecutablePath: installation.ExecutablePath}
		}
	}
	return batchEnergyPlusResolution{
		FailureStatus: "missing_energyplus",
		Error:         fmt.Sprintf("No compatible EnergyPlus installation is configured for input Version %d.%d.", requiredMajor, requiredMinor),
	}
}

func energyPlusMajorMinor(version string) (int, int, bool) {
	parts := versionNumbers(version)
	if len(parts) < 2 {
		return 0, 0, false
	}
	return parts[0], parts[1], true
}
