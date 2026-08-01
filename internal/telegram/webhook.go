package telegram

// WebhookPrefix is the fixed part of the bot callback path, tech.md §5.5.
const WebhookPrefix = "/webhooks/telegram/"

// WebhookPath builds the callback path Telegram is told to call. The path
// secret is part of the URL, so the result is a credential: it belongs in the
// setWebhook call and never in a log line.
func WebhookPath(pathSecret string) string {
	return WebhookPrefix + pathSecret
}
