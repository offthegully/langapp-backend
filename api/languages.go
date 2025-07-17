package api

import (
	"encoding/json"
	"net/http"

	"langapp-backend/languages"
)

type LanguagesResponse struct {
	Languages []languages.Language `json:"languages"`
}

func (api *APIService) GetLanguagesHandler(w http.ResponseWriter, r *http.Request) {
	response := LanguagesResponse{
		Languages: api.languagesService.GetSupportedLanguages(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
