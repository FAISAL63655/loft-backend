package cart

import (
	"encoding/json"
	"net/http"

	"encore.app/pkg/errs"
)

func writeError(w http.ResponseWriter, err *errs.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.HTTPStatus())
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    err.Code,
		"message": err.Message,
		"details": err.Details,
	})
}

//encore:api auth raw method=GET path=/cart/raw
func GetCartRaw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	res, err := GetCart(ctx)
	if err != nil {
		writeError(w, err.(*errs.Error))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
