package cities

import (
	"context"

	"encore.dev/storage/sqldb"

	"encore.app/pkg/errs"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct{}

func initService() (*Service, error) { return &Service{}, nil }

// City represents a city with its localized names and net shipping fee.
type City struct {
	ID             int64   `json:"id"`
	NameAR         string  `json:"name_ar"`
	NameEN         string  `json:"name_en"`
	ShippingFeeNet float64 `json:"shipping_fee_net"`
}

// CitiesResponse is the response type for listing enabled cities.
type CitiesResponse struct {
	Items []City `json:"items"`
}

// ListEnabledCities lists enabled cities with their localized names and net shipping fee.
//
//encore:api public method=GET path=/cities
func ListEnabledCities(ctx context.Context) (*CitiesResponse, error) {
	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT id, name_ar, name_en, shipping_fee_net
		FROM cities
		WHERE enabled = true
		ORDER BY id
	`)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل الاستعلام عن المدن"}
	}
	defer rows.Close()

	var items []City
	for rows.Next() {
		var c City
		if err := rows.Scan(&c.ID, &c.NameAR, &c.NameEN, &c.ShippingFeeNet); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة بيانات المدينة"}
		}
		items = append(items, c)
	}

	return &CitiesResponse{Items: items}, nil
}
