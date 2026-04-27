package cli

import "context"

func (a *App) runLicenseCheck(ctx context.Context, cfg LicenseConfig, runtimeMode RuntimeMode, documentType, action, runningMessage string, progress progressController) (*LicenseCheckResult, error) {
	emitProgress(ctx, progress, progressStepLicense, "running", runningMessage)
	result, err := a.checkLicenseWithRuntime(ctx, cfg, runtimeMode, documentType, action)
	if err != nil {
		emitProgress(ctx, progress, progressStepLicense, "failed", "Access check failed")
		return nil, err
	}
	emitProgress(ctx, progress, progressStepLicense, "completed", "Access check completed")
	return result, nil
}
