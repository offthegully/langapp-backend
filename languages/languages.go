package languages

type Language struct {
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
}

var SupportedLanguages = []Language{
	{Name: "English", ShortName: "EN"},
	{Name: "Spanish", ShortName: "ES"},
	{Name: "French", ShortName: "FR"},
	{Name: "German", ShortName: "DE"},
	{Name: "Italian", ShortName: "IT"},
	{Name: "Portuguese", ShortName: "PT"},
	{Name: "Russian", ShortName: "RU"},
	{Name: "Chinese", ShortName: "ZH"},
	{Name: "Japanese", ShortName: "JA"},
	{Name: "Korean", ShortName: "KO"},
	{Name: "Arabic", ShortName: "AR"},
	{Name: "Hindi", ShortName: "HI"},
	{Name: "Dutch", ShortName: "NL"},
	{Name: "Swedish", ShortName: "SV"},
	{Name: "Norwegian", ShortName: "NO"},
	{Name: "Danish", ShortName: "DA"},
	{Name: "Finnish", ShortName: "FI"},
	{Name: "Polish", ShortName: "PL"},
	{Name: "Czech", ShortName: "CS"},
	{Name: "Turkish", ShortName: "TR"},
}

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) GetSupportedLanguages() []Language {
	return SupportedLanguages
}

func (s *Service) IsValidLanguage(language string) bool {
	for _, lang := range SupportedLanguages {
		if lang.Name == language || lang.ShortName == language {
			return true
		}
	}
	return false
}

func GetSupportedLanguages() []Language {
	return SupportedLanguages
}

func IsValidLanguage(language string) bool {
	for _, lang := range SupportedLanguages {
		if lang.Name == language || lang.ShortName == language {
			return true
		}
	}
	return false
}