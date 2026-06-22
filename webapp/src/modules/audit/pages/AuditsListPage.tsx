// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { Button as MuiButton, Skeleton } from "@mui/material";
import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { Plus, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import { useNavigate } from "react-router";
import AuditCard from "@modules/audit/components/AuditCard";
import FilterPanel, { type FilterField } from "@modules/audit/components/FilterPanel";
import { useGetAudits } from "@modules/audit/api/useGetAudits";
import type { Audit } from "@modules/audit/types/audit";

const EMPTY_AUDIT_FILTERS: Record<string, string[]> = {
  status: [],
  framework: [],
  product: [],
};

function buildAuditFilterFields(audits: Audit[]): FilterField[] {
  const uniqueOptions = (getValue: (a: Audit) => string) => {
    const values = [...new Set(audits.map(getValue))].sort();
    return values.map((v) => ({ label: v, value: v }));
  };

  return [
    {
      id: "status",
      label: "Status",
      options: [
        { label: "Active", value: "ACTIVE" },
        { label: "Completed", value: "COMPLETED" },
        { label: "Archived", value: "ARCHIVED" },
      ],
    },
    {
      id: "framework",
      label: "Framework",
      options: uniqueOptions((a) => a.framework.name),
    },
    {
      id: "product",
      label: "Product",
      options: uniqueOptions((a) => a.product.name),
    },
  ];
}

function applyAuditFilters(
  audits: Audit[],
  filters: Record<string, string[]>,
  search: string,
): Audit[] {
  const q = search.trim().toLowerCase();
  return audits.filter((a) => {
    if (q && !a.name.toLowerCase().includes(q)) return false;
    if (filters.status?.length && !filters.status.includes(a.status)) return false;
    if (filters.framework?.length && !filters.framework.includes(a.framework.name)) return false;
    if (filters.product?.length && !filters.product.includes(a.product.name)) return false;
    return true;
  });
}

export default function AuditsListPage(): JSX.Element {
  const navigate = useNavigate();
  const { data, isLoading, isError, refetch } = useGetAudits();
  const [filters, setFilters] = useState<Record<string, string[]>>(EMPTY_AUDIT_FILTERS);
  const [search, setSearch] = useState("");

  const audits = data?.items ?? [];
  const filtered = applyAuditFilters(audits, filters, search);
  const filterFields = buildAuditFilterFields(audits);

  const anyActive =
    Object.values(filters).some((arr) => arr.length > 0) || search.trim().length > 0;

  return (
    <Box sx={{ p: { xs: 2, sm: 3 } }}>
      {/* Page header */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          mb: 3,
          flexWrap: "wrap",
          gap: 2,
        }}
      >
        <Typography variant="h5" fontWeight={700}>
          Audits
        </Typography>
        <Button
          variant="contained"
          startIcon={<Plus size={16} />}
          disabled
          sx={{ textTransform: "none" }}
        >
          New Audit
        </Button>
      </Box>

      {/* Search + Filter bar */}
      <Box sx={{ mb: 3 }}>
        <FilterPanel
          fields={filterFields}
          values={filters}
          onChange={setFilters}
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder="Search by audit name..."
        />
        {anyActive && (
          <Typography variant="caption" color="text.secondary" sx={{ mt: 0.75, display: "block" }}>
            {filtered.length} of {audits.length} audits
          </Typography>
        )}
      </Box>

      {/* Loading state */}
      {isLoading && (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
            gap: 2.5,
          }}
        >
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} variant="rectangular" height={220} sx={{ borderRadius: 2 }} />
          ))}
        </Box>
      )}

      {/* Error state */}
      {isError && (
        <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", py: 8, gap: 2 }}>
          <Typography variant="body1" color="text.secondary">
            Failed to load audits.
          </Typography>
          <MuiButton variant="outlined" onClick={() => void refetch()}>
            Try again
          </MuiButton>
        </Box>
      )}

      {/* Audit cards grid */}
      {!isLoading && !isError && filtered.length > 0 && (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
            gap: 2.5,
          }}
        >
          {filtered.map((audit) => (
            <AuditCard
              key={audit.id}
              audit={audit}
              onClick={() => void navigate(`/audit/audits/${audit.id}`)}
            />
          ))}
        </Box>
      )}

      {/* Empty state */}
      {!isLoading && !isError && filtered.length === 0 && (
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            py: 10,
            gap: 2,
          }}
        >
          <ShieldCheck size={48} style={{ opacity: 0.25 }} />
          <Typography variant="h6">No audits found</Typography>
          <Typography variant="body2" color="text.secondary">
            {anyActive
              ? "No audits match your search or filters."
              : "No audits have been created yet."}
          </Typography>
        </Box>
      )}
    </Box>
  );
}
