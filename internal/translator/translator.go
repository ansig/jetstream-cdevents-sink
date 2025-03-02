package translator

import (
	cdevents "github.com/cdevents/sdk-go/pkg/api"
)

type Webhook interface {
	Translate(data []byte) (cdevents.CDEvent, error)
}
