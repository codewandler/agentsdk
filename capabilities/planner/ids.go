package planner

import gonanoid "github.com/matoous/go-nanoid/v2"

func newPlanID() (string, error) {
	id, err := gonanoid.New(12)
	if err != nil {
		return "", err
	}
	return "plan_" + id, nil
}

func newStepID() (string, error) {
	id, err := gonanoid.New(12)
	if err != nil {
		return "", err
	}
	return "step_" + id, nil
}
