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
	languages, err := api.languagesService.GetSupportedLanguages()
	if err != nil {
		http.Error(w, "Failed to get supported languages", http.StatusInternalServerError)
		return
	}

	response := LanguagesResponse{
		Languages: languages,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
