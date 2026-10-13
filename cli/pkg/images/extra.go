// Source: getarcaneapp/arcane/cli/pkg/images/extra.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed standalone build workshop.

package images

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/image"
	"github.com/spf13/cobra"

	"github.com/florianfinn/docklet-next/cli/internal/cmdutil"
	"github.com/florianfinn/docklet-next/cli/internal/logger"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/prompt"
	"github.com/florianfinn/docklet-next/cli/internal/types"
)

var imagesSearchCmd = &cobra.Command{
	Use:          "search <term>",
	Short:        "Search Docker Hub images",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.GetLogger()
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		path := types.ImagesSearch(c.EnvID()) + "?term=" + url.QueryEscape(args[0])

		log.Debugf("Searching images from: %s", path)

		result, err := c.GetJSON[[]image.SearchResult](cmd.Context(), path)
		if err != nil {
			return fmt.Errorf("failed to search images: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) {
			return cmdutil.PrintJSON(result.Data)
		}

		headers := []string{"NAME", "DESCRIPTION", "STARS", "OFFICIAL"}
		var rows [][]string
		for _, item := range result.Data {
			official := ""
			if item.Official {
				official = "Yes"
			}
			rows = append(rows, []string{item.Name, truncateCell(item.Description, 60), strconv.Itoa(item.StarCount), official})
		}

		if tableErr := output.Table(headers, rows); tableErr != nil {
			return tableErr
		}
		output.Showing(len(result.Data), int64(len(result.Data)), "results")

		return nil
	},
}

var imagesHistoryCmd = &cobra.Command{
	Use:          "history <image-id|name>",
	Short:        "Show image layer history",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.GetLogger()
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		jsonOutput := cmdutil.JSONOutputEnabled(cmd)
		allowPrompt := !jsonOutput && prompt.IsInteractive()

		resolved, _, err := ImageRef.Resolve(cmd.Context(), c, args[0], allowPrompt)
		if err != nil {
			return err
		}
		imageID := resolved.ID
		path := types.ImageHistory(c.EnvID(), imageID)

		log.Debugf("Getting image history from: %s", path)

		resp, err := c.Get(cmd.Context(), path)
		if err != nil {
			return fmt.Errorf("failed to get image history: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := cmdutil.ReadJSONBody(resp)
		if err != nil {
			return fmt.Errorf("failed to get image history: %w", err)
		}

		if jsonOutput {
			return cmdutil.PrintRawJSON(body)
		}

		var result struct {
			Success bool                `json:"success"`
			Data    []image.HistoryItem `json:"data"`
		}
		if unmarshalErr := json.Unmarshal(body, &result); unmarshalErr != nil {
			return fmt.Errorf("failed to parse response: %w", unmarshalErr)
		}

		headers := []string{"CREATED", "CREATED BY", "SIZE", "COMMENT"}
		var rows [][]string
		for _, item := range result.Data {
			created := ""
			if item.Created > 0 {
				created = time.Unix(item.Created, 0).Format(time.RFC3339)
			}
			rows = append(rows, []string{created, truncateCell(item.CreatedBy, 60), output.Bytes(item.Size), truncateCell(item.Comment, 30)})
		}

		return output.Table(headers, rows)
	},
}

var imagesTagCmd = &cobra.Command{
	Use:          "tag <image-id|name> <new-ref>",
	Short:        "Add a repository tag to an image",
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.GetLogger()
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		jsonOutput := cmdutil.JSONOutputEnabled(cmd)
		allowPrompt := !jsonOutput && prompt.IsInteractive()

		resolved, _, err := ImageRef.Resolve(cmd.Context(), c, args[0], allowPrompt)
		if err != nil {
			return err
		}
		imageID := resolved.ID

		repository, tag := splitImageRef(args[1])
		if repository == "" {
			return errors.New("target repository is required")
		}

		path := types.ImageTag(c.EnvID(), imageID)

		log.Debugf("Tagging image via: %s", path)

		resp, err := c.Post(cmd.Context(), path, image.TagRequest{Repository: repository, Tag: tag})
		if err != nil {
			return fmt.Errorf("failed to tag image: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := cmdutil.ReadJSONBody(resp)
		if err != nil {
			return fmt.Errorf("failed to tag image: %w", err)
		}

		if jsonOutput {
			return cmdutil.PrintRawJSON(body)
		}

		var result struct {
			Success bool                 `json:"success"`
			Data    base.MessageResponse `json:"data"`
		}
		if unmarshalErr := json.Unmarshal(body, &result); unmarshalErr != nil {
			return fmt.Errorf("failed to parse response: %w", unmarshalErr)
		}

		output.Success("%s", result.Data.Message)

		return nil
	},
}

var imagesExportCmd = &cobra.Command{
	Use:          "export <image-id|name> [output-file]",
	Short:        "Download an image as a tar archive",
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.GetLogger()
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		// Exporting large images can take a long time
		c.SetTimeout(30 * time.Minute)

		allowPrompt := prompt.IsInteractive()

		resolved, _, err := ImageRef.Resolve(cmd.Context(), c, args[0], allowPrompt)
		if err != nil {
			return err
		}
		imageID := resolved.ID

		outputFile := exportFileName(args[0])
		if len(args) == 2 {
			outputFile = args[1]
		}

		path := types.ImageExport(c.EnvID(), imageID)

		log.Debugf("Exporting image from: %s", path)

		resp, err := c.RequestRaw(cmd.Context(), http.MethodGet, path, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to export image: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if ensureSuccessStatusErr := cmdutil.EnsureSuccessStatus(resp); ensureSuccessStatusErr != nil {
			return fmt.Errorf("failed to export image: %w", ensureSuccessStatusErr)
		}

		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}

		written, err := output.Copy(cmd.Context(), file, resp.Body, resp.ContentLength, "Exporting image", !cmdutil.JSONOutputEnabled(cmd))
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(outputFile)
			return fmt.Errorf("failed to write image archive: %w", err)
		}

		output.Success("Saved %s (%s)", outputFile, output.Bytes(written))

		return nil
	},
}

var (
	attestationsPlatform      string
	attestationsPredicateType string
	attestationsStatement     bool
)

var imagesAttestationsCmd = &cobra.Command{
	Use:          "attestations <image>",
	Short:        "Show in-toto attestations attached to an image",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.GetLogger()
		c, err := cmdutil.ClientFromCommand(cmd)
		if err != nil {
			return err
		}

		imageName := strings.TrimSpace(args[0])
		if imageName == "" {
			return errors.New("image identifier is required")
		}

		u, err := url.Parse(types.ImageAttestations(c.EnvID(), imageName))
		if err != nil {
			return fmt.Errorf("failed to parse endpoint path: %w", err)
		}
		q := u.Query()
		if attestationsPlatform != "" {
			q.Set("platform", attestationsPlatform)
		}
		if attestationsPredicateType != "" {
			q.Set("predicateType", attestationsPredicateType)
		}
		if attestationsStatement {
			q.Set("statement", "true")
		}
		u.RawQuery = q.Encode()
		path := u.String()

		log.Debugf("Getting image attestations from: %s", path)

		resp, err := c.Get(cmd.Context(), path)
		if err != nil {
			return fmt.Errorf("failed to get image attestations: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := cmdutil.ReadJSONBody(resp)
		if err != nil {
			return fmt.Errorf("failed to get image attestations: %w", err)
		}

		if cmdutil.JSONOutputEnabled(cmd) || attestationsStatement {
			return cmdutil.PrintRawJSON(body)
		}

		var result struct {
			Success bool                  `json:"success"`
			Data    image.AttestationList `json:"data"`
		}
		if unmarshalErr := json.Unmarshal(body, &result); unmarshalErr != nil {
			return fmt.Errorf("failed to parse response: %w", unmarshalErr)
		}

		output.Header("Image Attestations")
		output.KeyValue("Image", result.Data.ImageRef)
		output.KeyValue("Subject Digest", result.Data.SubjectDigest)
		if result.Data.Platform != "" {
			output.KeyValue("Platform", result.Data.Platform)
		}

		if len(result.Data.Attestations) == 0 {
			output.Info("No attestations found")
			return nil
		}

		headers := []string{"PREDICATE TYPE", "PLATFORM", "DIGEST", "SIZE"}
		var rows [][]string
		for _, att := range result.Data.Attestations {
			rows = append(rows, []string{att.PredicateType, att.Platform, truncateCell(att.Digest, 19), output.Bytes(att.Size)})
		}
		_, _ = fmt.Fprintln(output.Stdout())
		if tableErr := output.Table(headers, rows); tableErr != nil {
			return tableErr
		}

		return nil
	},
}

// splitImageRef splits an image reference into repository and tag. The tag is
// only split off when the text after the final colon contains no slash, so
// registry ports (registry:5000/repo) stay part of the repository.
func splitImageRef(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if idx := strings.LastIndex(ref, ":"); idx > 0 && !strings.Contains(ref[idx+1:], "/") {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

// exportFileName mirrors the server's default export filename for an image
// reference, replacing separators with underscores.
func exportFileName(imageName string) string {
	name := strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(strings.TrimSpace(imageName))
	name = cmp.Or(strings.Trim(name, "._-"), "image")
	return name + ".tar"
}

func truncateCell(value string, limit int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if limit > 3 && len(value) > limit {
		return value[:limit-3] + "..."
	}
	return value
}

func init() {
	ImagesCmd.AddCommand(imagesSearchCmd)

	ImagesCmd.AddCommand(imagesHistoryCmd)

	ImagesCmd.AddCommand(imagesTagCmd)

	ImagesCmd.AddCommand(imagesExportCmd)

	ImagesCmd.AddCommand(imagesAttestationsCmd)
	imagesAttestationsCmd.Flags().StringVar(&attestationsPlatform, "platform", "", "OCI platform selector (e.g. linux/amd64)")
	imagesAttestationsCmd.Flags().StringVar(&attestationsPredicateType, "predicate-type", "", "Exact in-toto predicate type URI to include")
	imagesAttestationsCmd.Flags().BoolVar(&attestationsStatement, "statement", false, "Include verbatim statement JSON bodies (implies JSON output)")

}
