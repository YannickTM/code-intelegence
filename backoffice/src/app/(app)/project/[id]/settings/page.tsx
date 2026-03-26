"use client";

import { useParams } from "next/navigation";
import { useProjectDetail } from "~/hooks/use-project-detail";
import { GeneralSettingsSection } from "~/components/project-detail/settings-general";
import { SSHKeySettingsSection } from "~/components/project-detail/settings-ssh-key";
import { MembersSection } from "~/components/project-detail/settings-members";
import { ProjectApiKeysSection } from "~/components/project-detail/settings-api-keys";
import { EmbeddingSettingsSection } from "~/components/project-detail/settings-embedding";
import { LLMSettingsSection } from "~/components/project-detail/settings-llm";
import { DangerZoneSection } from "~/components/project-detail/settings-danger-zone";

export default function ProjectSettingsPage() {
  const params = useParams<{ id: string }>();
  const { project, isAdminOrOwner } = useProjectDetail(params.id);

  if (!project) return null; // layout handles loading/error

  return (
    <div className="flex max-w-2xl flex-col gap-8">
      <GeneralSettingsSection
        project={project}
        projectId={params.id}
        role={project.role}
      />
      {isAdminOrOwner && <SSHKeySettingsSection projectId={params.id} />}
      <EmbeddingSettingsSection projectId={params.id} role={project.role} />
      <LLMSettingsSection projectId={params.id} role={project.role} />

      <MembersSection projectId={params.id} role={project.role} />

      <ProjectApiKeysSection projectId={params.id} role={project.role} />

      <DangerZoneSection project={project} />
    </div>
  );
}
