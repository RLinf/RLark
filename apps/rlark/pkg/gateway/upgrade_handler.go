package gateway

import (
	"encoding/json"
	"errors"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// prepareUpgradeJobForCreate sets GenerateName and validates the image before
// the generic create handler persists the object.
func prepareUpgradeJobForCreate(body []byte) ([]byte, error) {
	var job rlarkv1alpha1.UpgradeJob
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, err
	}

	if job.Spec.Image == "" {
		return nil, errors.New("spec.image is required")
	}

	job.GenerateName = "up-"
	if job.Annotations == nil {
		job.Annotations = map[string]string{}
	}

	return json.Marshal(&job)
}
