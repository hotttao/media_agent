package execution

import "errors"

func ValidateProductionParameters(model string, seconds int, ratio string) error {
	models := map[string]bool{"seedance-2.5": true, "seedance-2.0": true, "seedance-2.0-fast": true, "seedance-2.0-mini": true}
	ratios := map[string]bool{"1:1": true, "3:4": true, "4:3": true, "9:16": true, "16:9": true, "21:9": true}
	if !models[model] || seconds < 4 || seconds > 15 || !ratios[ratio] {
		return errors.New("unsupported studio model/duration/aspect ratio")
	}
	return nil
}
