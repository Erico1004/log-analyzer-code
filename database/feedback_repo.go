package database

import "log-analyzer/model"

type FeedbackRepo struct{}

func NewFeedbackRepo() *FeedbackRepo {
	return &FeedbackRepo{}
}

func (r *FeedbackRepo) Create(feedback *model.UserFeedback) error {
	return DB.Create(feedback).Error
}
