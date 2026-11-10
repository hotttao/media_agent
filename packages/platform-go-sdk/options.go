package platformsdk

// Config contains server-only application credentials. BaseURL is the platform public gateway.
type Config struct{ BaseURL, ClientID, ClientSecret string }
