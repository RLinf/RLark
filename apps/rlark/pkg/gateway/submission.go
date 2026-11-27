package gateway

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/pkg/submission"
)

type workloadSubmitter struct {
	username string
	id       string
}

func (g *Gateway) verifiedSubmitter(c *gin.Context, resource string) (workloadSubmitter, error) {
	if resource != "jobs" && resource != "workflows" {
		return workloadSubmitter{}, nil
	}
	subject := c.GetString(authSubjectKey)
	account, machine := strings.CutPrefix(subject, serviceAccountSubject)
	raw := c.GetHeader(submission.Header)
	if raw == "" {
		if machine {
			return workloadSubmitter{}, nil
		}
		return workloadSubmitter{username: subject}, nil
	}
	if len(g.submissionKey) == 0 || !machine ||
		(account != "astribot-iam-admin" && account != "astribot-iam-operator") {
		return workloadSubmitter{}, errors.New("workload submission assertion requires an IAM service account")
	}
	claims, err := submission.Verify(g.submissionKey, raw, account, c.Request.URL.Path)
	if err != nil {
		return workloadSubmitter{}, err
	}
	return workloadSubmitter{username: claims.Username, id: claims.Subject}, nil
}
