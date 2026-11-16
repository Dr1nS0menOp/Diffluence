-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 074, wave GT5: the pages made from templates, for the filter template: of search
-- (TPL-703). The templates module owns the origin of a page; search asks this function for
-- the pages of the templates a reader may see, and every result still passes
-- perm_visible_pages. It reads through the indexes on template_sets (template_id, version)
-- and page_template_origin (set_id). The previous release does not call it.
CREATE OR REPLACE FUNCTION template_origin_pages(template_ids uuid[]) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT o.page_id
    FROM template_sets s
    JOIN page_template_origin o ON o.set_id = s.id
    WHERE s.template_id = ANY (template_ids)
$$;
