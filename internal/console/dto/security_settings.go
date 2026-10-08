package dto

type SecuritySettingsView struct {
	ActivationLifetimeHours int  `json:"activationLifetimeHours"`
	PasswordLifetimeDays    int  `json:"passwordLifetimeDays"`
	PasswordMinLength       int  `json:"passwordMinLength"`
	PasswordHistoryCount    int  `json:"passwordHistoryCount"`
	MaxFailedLoginAttempts  int  `json:"maxFailedLoginAttempts"`
	LockoutDurationMinutes  int  `json:"lockoutDurationMinutes"`
	SessionIdleMinutes      int  `json:"sessionIdleMinutes"`
	SessionAbsoluteMinutes  int  `json:"sessionAbsoluteMinutes"`
	AllowConcurrentSessions bool `json:"allowConcurrentSessions"`
}

type SecuritySettingsInput = SecuritySettingsView
