package server

import (
	"context"

	"github.com/noonat/eyeball/internal/store"
)

// db is the only way the server reaches the store.
//
// Every method that writes publishes on the hub after it succeeds, so a request
// waiting on a review wakes whichever handler made the change. The bare store
// is an unexported field rather than an embedded one: embedding would promote
// its write methods, and a handler calling one of those directly would leave a
// waiting agent asleep on a review that had already been answered.
type db struct {
	s   *store.Store
	hub *hub
}

// publish signals the hub when a write succeeded, and passes its error on.
func (d *db) publish(err error) error {
	if err == nil {
		d.hub.publish()
	}
	return err
}

// EnsureProject registers a project. It changes nothing a waiter reads, so it
// does not publish.
func (d *db) EnsureProject(ctx context.Context, root, name string) (store.Project, error) {
	return d.s.EnsureProject(ctx, root, name)
}

// MarkInsideCheckout records that a .git was seen at or above a marker
// project's root. It does not publish: nothing a waiting agent reads changes,
// and a file page's GET writes it, so once the live stream reloads a page on
// every publish, a page that published here would reload itself in a loop.
func (d *db) MarkInsideCheckout(ctx context.Context, projectID int64) error {
	return d.s.MarkInsideCheckout(ctx, projectID)
}

// OpenReview opens a review and its first round, then publishes.
func (d *db) OpenReview(ctx context.Context, nr store.NewReview) (store.Review, store.Round, error) {
	review, round, err := d.s.OpenReview(ctx, nr)
	return review, round, d.publish(err)
}

// OpenRound opens a review's next round, then publishes.
func (d *db) OpenRound(ctx context.Context, reviewID int64, req store.Request) (store.Round, error) {
	round, err := d.s.OpenRound(ctx, reviewID, req)
	return round, d.publish(err)
}

// Abandon withdraws a review, then publishes.
func (d *db) Abandon(ctx context.Context, reviewID int64) error {
	return d.publish(d.s.Abandon(ctx, reviewID))
}

// AddComment stores a comment, then publishes.
func (d *db) AddComment(ctx context.Context, nc store.NewComment) (store.Comment, error) {
	c, err := d.s.AddComment(ctx, nc)
	return c, d.publish(err)
}

// EditComment replaces a comment's body, then publishes.
func (d *db) EditComment(ctx context.Context, commentID int64, body string) error {
	return d.publish(d.s.EditComment(ctx, commentID, body))
}

// DeleteComment removes a comment, then publishes.
func (d *db) DeleteComment(ctx context.Context, commentID int64) error {
	return d.publish(d.s.DeleteComment(ctx, commentID))
}

// Decide records a verdict, then publishes.
func (d *db) Decide(ctx context.Context, roundID int64, verdict store.Verdict, note string) error {
	return d.publish(d.s.Decide(ctx, roundID, verdict, note))
}

// MarkDelivered stamps comments as received, then publishes.
func (d *db) MarkDelivered(ctx context.Context, reviewID int64, ids []int64) error {
	return d.publish(d.s.MarkDelivered(ctx, reviewID, ids))
}

// MarkVerdictDelivered stamps a verdict as received, then publishes.
func (d *db) MarkVerdictDelivered(ctx context.Context, reviewID int64, round int) error {
	return d.publish(d.s.MarkVerdictDelivered(ctx, reviewID, round))
}

// Reply stores an agent's reply, then publishes.
func (d *db) Reply(ctx context.Context, commentID int64, body string) error {
	return d.publish(d.s.Reply(ctx, commentID, body))
}

// Review reads one review.
func (d *db) Review(ctx context.Context, reviewID int64) (store.Review, error) {
	return d.s.Review(ctx, reviewID)
}

// Round reads one round by number.
func (d *db) Round(ctx context.Context, reviewID int64, number int) (store.Round, error) {
	return d.s.Round(ctx, reviewID, number)
}

// RoundFiles reads what a round captured.
func (d *db) RoundFiles(ctx context.Context, roundID int64) ([]store.RoundFile, error) {
	return d.s.RoundFiles(ctx, roundID)
}

// Decision reads the verdict on a round.
func (d *db) Decision(ctx context.Context, roundID int64) (store.Decision, error) {
	return d.s.Decision(ctx, roundID)
}

// Released reads the comments an agent may receive.
func (d *db) Released(ctx context.Context, reviewID int64) ([]store.ReleasedComment, error) {
	return d.s.Released(ctx, reviewID)
}

// ReviewsFor lists one agent's reviews in a project.
func (d *db) ReviewsFor(ctx context.Context, projectID int64, agent string) ([]store.ReviewRow, error) {
	return d.s.ReviewsFor(ctx, projectID, agent)
}

// ReviewsIn lists every review in a project.
func (d *db) ReviewsIn(ctx context.Context, projectID int64) ([]store.ReviewRow, error) {
	return d.s.ReviewsIn(ctx, projectID)
}

// ProjectByRoot reads the project at a root, if it has been seen.
func (d *db) ProjectByRoot(ctx context.Context, root string) (store.Project, error) {
	return d.s.ProjectByRoot(ctx, root)
}

// Project reads one project.
func (d *db) Project(ctx context.Context, projectID int64) (store.Project, error) {
	return d.s.Project(ctx, projectID)
}

// CommentReview reads which review a comment belongs to.
func (d *db) CommentReview(ctx context.Context, commentID int64) (int64, error) {
	return d.s.CommentReview(ctx, commentID)
}
