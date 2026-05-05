// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ArchiveDomainParams struct {
	DomainID string `json:"domain_id" jsonschema:"ID du domaine à archiver"`
}

func registerArchiveDomain(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "archive_domain",
		Description: "Archive un domaine — il disparaît du cockpit et du routing mais la progression est préservée. Utiliser unarchive_domain pour le réactiver.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params ArchiveDomainParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("archive_domain: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}
		if params.DomainID == "" {
			r, _ := errorResult("domain_id is required")
			return r, nil, nil
		}

		// Verify domain exists and belongs to learner
		domain, err := deps.Store.GetDomainByID(params.DomainID)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("domain not found: %s", params.DomainID))
			return r, nil, nil
		}
		if domain.LearnerID != learnerID {
			r, _ := errorResult("domain not found")
			return r, nil, nil
		}

		if err := deps.Store.ArchiveDomain(params.DomainID, learnerID); err != nil {
			deps.Logger.Error("archive_domain: failed", "err", err, "domain", params.DomainID)
			r, _ := errorResult(fmt.Sprintf("failed to archive domain: %v", err))
			return r, nil, nil
		}

		deps.Logger.Info("archive_domain: success", "domain", params.DomainID, "name", domain.Name, "learner", learnerID)
		r, _ := jsonResult(map[string]interface{}{
			"archived":    true,
			"domain_id":   domain.ID,
			"domain_name": domain.Name,
			"message":     fmt.Sprintf("Domaine '%s' archivé. La progression est préservée. Utilise unarchive_domain pour le réactiver.", domain.Name),
		})
		return r, nil, nil
	})
}

type UnarchiveDomainParams struct {
	DomainID string `json:"domain_id" jsonschema:"ID du domaine à réactiver"`
}

func registerUnarchiveDomain(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "unarchive_domain",
		Description: "Réactive un domaine archivé — il réapparaît dans le cockpit et le routing avec toute sa progression préservée.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params UnarchiveDomainParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("unarchive_domain: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}
		if params.DomainID == "" {
			r, _ := errorResult("domain_id is required")
			return r, nil, nil
		}

		domain, err := deps.Store.GetDomainByID(params.DomainID)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("domain not found: %s", params.DomainID))
			return r, nil, nil
		}
		if domain.LearnerID != learnerID {
			r, _ := errorResult("domain not found")
			return r, nil, nil
		}

		if err := deps.Store.UnarchiveDomain(params.DomainID, learnerID); err != nil {
			deps.Logger.Error("unarchive_domain: failed", "err", err, "domain", params.DomainID)
			r, _ := errorResult(fmt.Sprintf("failed to unarchive domain: %v", err))
			return r, nil, nil
		}

		deps.Logger.Info("unarchive_domain: success", "domain", params.DomainID, "name", domain.Name, "learner", learnerID)
		r, _ := jsonResult(map[string]interface{}{
			"archived":    false,
			"domain_id":   domain.ID,
			"domain_name": domain.Name,
			"message":     fmt.Sprintf("Domaine '%s' réactivé.", domain.Name),
		})
		return r, nil, nil
	})
}

type DeleteDomainParams struct {
	DomainID string `json:"domain_id" jsonschema:"ID du domaine à supprimer définitivement"`
	Confirm  bool   `json:"confirm" jsonschema:"Doit être true pour confirmer la suppression"`
}

func registerDeleteDomain(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_domain",
		Description: "Supprime définitivement un domaine. Les concept_states et interactions sont préservés. Nécessite confirm=true.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params DeleteDomainParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("delete_domain: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}
		if params.DomainID == "" {
			r, _ := errorResult("domain_id is required")
			return r, nil, nil
		}
		if !params.Confirm {
			r, _ := errorResult("confirm must be true to delete a domain. This action is irreversible.")
			return r, nil, nil
		}

		domain, err := deps.Store.GetDomainByID(params.DomainID)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("domain not found: %s", params.DomainID))
			return r, nil, nil
		}
		if domain.LearnerID != learnerID {
			r, _ := errorResult("domain not found")
			return r, nil, nil
		}

		if err := deps.Store.DeleteDomain(params.DomainID, learnerID); err != nil {
			deps.Logger.Error("delete_domain: failed", "err", err, "domain", params.DomainID)
			r, _ := errorResult(fmt.Sprintf("failed to delete domain: %v", err))
			return r, nil, nil
		}

		deps.Logger.Info("delete_domain: success", "domain", params.DomainID, "name", domain.Name, "learner", learnerID)
		r, _ := jsonResult(map[string]interface{}{
			"deleted":     true,
			"domain_id":   domain.ID,
			"domain_name": domain.Name,
			"message":     fmt.Sprintf("Domaine '%s' supprime definitivement. Les concept_states et l'historique d'interactions sont preserves.", domain.Name),
		})
		return r, nil, nil
	})
}
