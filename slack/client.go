package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/schema"
)

type SlackMethod string

func (m SlackMethod) String() string {
	return string(m)
}

type ContentType string

func (m ContentType) String() string {
	return string(m)
}

type Header string

func (m Header) String() string {
	return string(m)
}

const (
	apiURL                            = "https://slack.com/api"
	MethodChatPostMessage SlackMethod = "/chat.postMessage"
	MethodUsersInfo       SlackMethod = "/users.info"
	ContentTypeJSON       ContentType = "application/json"
	ContentTypeForm       ContentType = "application/x-www-form-urlencoded"
	HeaderContentType     Header      = "Content-Type"
	HeaderAuthorization   Header      = "Authorization"
)

type Client struct {
	token string
}

func New(token string) *Client {
	return &Client{token: token}
}

type ChatPostMessageRequest struct {
	// Channel, private group, or IM channel to send message to.
	// Can be an encoded ID, or a name.
	Channel string `json:"channel"`
	// Attachments/Blocks/Text
	// One of these arguments is required to describe the content of the message.
	// If attachments or blocks are included, text will be used as fallback text for
	// notifications only.
	// A JSON-based array of structured attachments, presented as a URL-encoded string.
	// Example: `[{"pretext": "pre-hello", "text": "text-world"}]`
	Attachments string `json:"attachments,omitempty"`
	// Attachments/Blocks/Text
	// One of these arguments is required to describe the content of the message.
	// If attachments or blocks are included, text will be used as fallback text for
	// notifications only.
	// A JSON-based array of structured blocks, presented as a URL-encoded string.
	// Example:
	// `[{"type": "section", "text": {"type": "plain_text", "text": "Hello world"}}]`
	Blocks string `json:"blocks,omitempty"`
	// Attachments/Blocks/Text
	// One of these arguments is required to describe the content of the message.
	// If attachments or blocks are included, text will be used as fallback text for
	// notifications only.
	// Example: "Hello world"
	Text string `json:"text,omitempty"`
	// Pass true to post the message as the authed user, instead of as a bot.
	// Defaults to false.
	AsUser bool `json:"as_user,omitempty"`
	// Emoji to use as the icon for this message.
	// Overrides icon_url.
	// Must be used in conjunction with as_user set to false, otherwise ignored.
	// Example: ":chart_with_upwards_trend:"
	IconEmoji string `json:"icon_emoji,omitempty"`
	// URL to an image to use as the icon for this message.
	// Must be used in conjunction with as_user set to false, otherwise ignored.
	// Example: "http://lorempixel.com/48/48"
	IconURL string `json:"icon_url,omitempty"`
	// Find and link channel names and usernames.
	LinkNames bool `json:"link_names,omitempty"`
	// Disable Slack markup parsing by setting to false.
	// Note: This is DISABLED by default in the Go API due to the zero value of a bool
	// being "false".
	// Defaults to false.
	Mrkdwn bool `json:"mrkdwn"` // Note: no omitempty because Slack API treats empty as true
	// Change how messages are treated.
	// Defaults to "none".
	Parse string `json:"parse,omitempty"`
	// Used in conjunction with thread_ts and indicates whether reply should be made
	// visible to everyone in the channel or conversation.
	// Defaults to false.
	ReplyBroadcast bool `json:"reply_broadcast,omitempty"`
	// Provide another message's ts value to make this message a reply.
	// Avoid using a reply's ts value; use its parent instead.
	ThreadTS string `json:"thread_ts,omitempty"`
	// Pass true to enable unfurling of primarily text-based content.
	// Defaults to false.
	UnfurlLinks bool `json:"unfurl_links,omitempty"`
	// Pass false to disable unfurling of media content.
	// Defaults to false.
	UnfurlMedia bool `json:"unfurl_media,omitempty"`
	// Set your bot's user name.
	// Must be used in conjunction with as_user set to false, otherwise ignored.
	Username string `json:"username,omitempty"`
}

func NewChatPostMessageRequest(channel string) *ChatPostMessageRequest {
	return &ChatPostMessageRequest{Channel: channel, Mrkdwn: true}
}

type ChatPostMessageResponse struct {
	OK               bool                   `json:"ok"`
	Channel          string                 `json:"channel,omitempty"`
	Error            string                 `json:"error,omitempty"`
	Message          map[string]interface{} `json:"message,omitempty"`
	TS               string                 `json:"ts,omitempty"`
	Warning          string                 `json:"warning,omitempty"`
	ResponseMetadata struct {
		Warnings []string `json:"warnings,omitempty"`
	} `json:"response_metadata,omitempty"`
}

func (c *ChatPostMessageResponse) IsOK() bool {
	return c.OK
}

func (c *ChatPostMessageResponse) GetError() string {
	return c.Error
}

type Response interface {
	IsOK() bool
	GetError() string
}

func (c *Client) doRequest(request *http.Request, response Response, method SlackMethod) error {
	request.Header.Set(HeaderAuthorization.String(), fmt.Sprintf("Bearer %s", c.token))

	httpClient := &http.Client{}
	httpResponse, err := httpClient.Do(request)

	if err != nil {
		return err
	}

	// Make sure the requests was sucessful and log the response if the request failed.
	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("Received non-success response from Slack API: %s", httpResponse.Status)
	}

	decoder := json.NewDecoder(httpResponse.Body)
	if err := decoder.Decode(response); err != nil {
		return err
	}

	if !response.IsOK() {
		return fmt.Errorf("Received error response from Slack API. See https://api.slack.com/methods%s#errors for more info. Error: %s", method, response.GetError())
	}

	return nil
}

// https://api.slack.com/methods/chat.postMessage
func (c *Client) ChatPostMessage(request *ChatPostMessageRequest) (*ChatPostMessageResponse, error) {
	requestBody, err := json.MarshalIndent(&request, "", "  ")

	// Set a context with a 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate the request.
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		apiURL+MethodChatPostMessage.String(),
		bytes.NewBuffer(requestBody),
	)

	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set(HeaderContentType.String(), ContentTypeJSON.String())
	response := &ChatPostMessageResponse{}
	return response, c.doRequest(httpRequest, response, MethodChatPostMessage)
}

type UsersInfoRequest struct {
	// User to get info on
	// Example: "W1234567890"
	User string `schema:"user,required"`
	// Set this to true to receive the locale for this user.
	// Defaults to false.
	IncludeLocale bool `schema:"include_locale"`
}

func NewUsersInfoRequest(user string) *UsersInfoRequest {
	return &UsersInfoRequest{User: user}
}

type UsersInfoResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	User  struct {
		// Indicates that a bot user is set to be constantly active in presence status.
		AlwaysActive bool `json:"always_active,omitempty"`
		// Used in some clients to display a special username color.
		Color string `json:"color,omitempty"`
		// This user has been deactivated when the value of this field is true.
		// Otherwise the value is false, or the field may not appear at all.
		Deleted bool `json:"deleted,omitempty"`
		// An object containing info related to an Enterprise Grid user.
		EnterpriseUser struct {
			// A unique ID for the Enterprise Grid organization this user belongs to.
			EnterpriseID string `json:"enterprise_id,omitempty"`
			// A display name for the Enterprise Grid organization.
			EnterpriseName string `json:"enterprise_name,omitempty"`
			// This user's ID - some Grid users have a kind of dual identity — a local,
			// workspace-centric user ID as well as a Grid-wise user ID, called the
			// Enterprise user ID.
			// In most cases these IDs can be used interchangeably, but when it is
			// provided, we strongly recommend using this Enterprise user id over the
			// root level user id field.
			ID string `json:"id,omitempty"`
			// Indicates whether the user is an Admin of the Enterprise Grid
			// organization.
			IsAdmin bool `json:"is_admin,omitempty"`
			// Indicates whether the user is an Owner of the Enterprise Grid
			// organization.
			IsOwner bool `json:"is_owner,omitempty"`
			// An array of workspace IDs that are in the Enterprise Grid organization.
			Teams []string `json:"teams,omitempty"`
		} `json:"enterprise_user,omitempty"`
		// Identifier for this workspace user.
		// It is unique to the workspace containing the user.
		// Use this field together with team_id as a unique key when storing related
		// data or when specifying the user in API requests. We recommend considering
		// the format of the string to be an opaque value, and not to rely on a
		// particular structure.
		ID string `json:"id,omitempty"`
		// Indicates whether the user is an Admin of the current workspace.
		IsAdmin           bool   `json:"is_admin,omitempty"`
		IsAppUser         bool   `json:"is_app_user,omitempty"`
		IsBot             bool   `json:"is_bot,omitempty"`
		IsEmailConfirmed  bool   `json:"is_email_confirmed,omitempty"`
		IsOwner           bool   `json:"is_owner,omitempty"`
		IsPrimaryOwner    bool   `json:"is_primary_owner,omitempty"`
		IsRestricted      bool   `json:"is_restricted,omitempty"`
		IsUltraRestricted bool   `json:"is_ultra_restricted,omitempty"`
		Name              string `json:"name,omitempty"`

		// Describes whether two-factor authentication is enabled for this user.
		Has2FA bool `json:"has_2fa,omitempty"`
		// An object containing the default fields of a user's workspace profile.
		Profile struct {
			AvatarHash string `json:"avatar_hash,omitempty"`
			// Indicates the display name that the user has chosen to identify
			// themselves by in their workspace profile. Do not use this field as a
			// unique identifier for a user, as it may change at any time.
			// Instead, use id and team_id in concert.
			DisplayName string `json:"display_name,omitempty"`
			// The display_name field, but with any non-Latin characters filtered out.
			DisplayNameNormalized string `json:"display_name_normalized,omitempty"`
			FirstName             string `json:"first_name"`
			Image24               string `json:"image_24,omitempty"`
			Image32               string `json:"image_32,omitempty"`
			Image48               string `json:"image_48,omitempty"`
			Image72               string `json:"image_72,omitempty"`
			Image192              string `json:"image_192,omitempty"`
			Image512              string `json:"image_512,omitempty"`
			Image1024             string `json:"image_1024,omitempty"`
			ImageOriginal         string `json:"image_original,omitempty"`
			IsCustomImage         bool   `json:"is_custom_image,omitempty"`
			LastName              string `json:"last_name,omitempty"`
			Phone                 string `json:"phone,omitempty"`
			Pronouns              string `json:"pronouns,omitempty"`
			// The real name that the user specified in their workspace profile.
			RealName string `json:"real_name,omitempty"`
			// The real_name field, but with any non-Latin characters filtered out.
			RealNameNomralized     string   `json:"real_name_normalized,omitempty"`
			Skype                  string   `json:"skype,omitempty"`
			StatusEmoji            string   `json:"status_emoji,omitempty"`
			StatusEmojiDisplayInfo []string `json:"status_emoji_display_info,omitempty"`
			StatusExpiration       int64    `json:"status_expiration,omitempty"`
			StatusText             string   `json:"status_text,omitempty"`
			StatusTextCanonical    string   `json:"status_text_canonical,omitempty"`
			Team                   string   `json:"team,omitempty"`
			Title                  string   `json:"title,omitempty"`
		} `json:"profile,omitempty"`
		RealName string `json:"real_name,omitempty"`
		TeamID   string `json:"team_id"`
		// Indicates the type of two-factor authentication in use.
		// Only present if has_2fa is true.
		// The value will be either "app" or "sms".
		TwoFactorType string `json:"two_factor_type,omitempty"`
		// A human-readable string for the geographic timezone-related region this user
		// has specified in their account.
		TZ string `json:"tz,omitempty"`
		// Describes the commonly used name of the tz timezone.
		TZLabel string `json:"tz_label,omitempty"`
		// Indicates the number of seconds to offset UTC time by for this user's tz.
		TZOffset int `json:"tz_offset,omitempty"`
		// A unix timestamp indicating when the user object was last updated.
		Updated                int64  `json:"updated,omitempty"`
		WhoCanShareContactCard string `json:"who_can_share_contact_card,omitempty"`
	} `json:"user,omitempty"`
	ResponseMetadata struct {
		Warnings []string `json:"warnings,omitempty"`
	} `json:"response_metadata,omitempty"`

	Warning string `json:"warning,omitempty"`
}

func (c *UsersInfoResponse) IsOK() bool {
	return c.OK
}

func (c *UsersInfoResponse) GetError() string {
	return c.Error
}

// https://api.slack.com/methods/users.info
func (c *Client) UsersInfo(request *UsersInfoRequest) (*UsersInfoResponse, error) {
	values := url.Values{}
	encoder := schema.NewEncoder()
	if err := encoder.Encode(request, values); err != nil {
		return nil, fmt.Errorf("Failed to encode url params: %w", err)
	}

	// Set a context with a 10s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate the request.
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		apiURL+MethodUsersInfo.String(),
		nil,
	)

	if err != nil {
		return nil, err
	}

	httpRequest.Header.Set(HeaderContentType.String(), ContentTypeForm.String())
	httpRequest.URL.RawQuery = values.Encode()

	response := &UsersInfoResponse{}
	if err := c.doRequest(httpRequest, response, MethodUsersInfo); err != nil {
		return response, fmt.Errorf("HTTP request returned an error: %w", err)
	}

	return response, nil
}
