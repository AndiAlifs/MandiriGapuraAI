using GapuraAI.API.Models;
using Microsoft.EntityFrameworkCore;

namespace GapuraAI.API.Data;

/// <summary>
/// Entity Framework Core DbContext for the GAPURA AI Studio database.
/// Configured for MySQL 8.0 via the Pomelo.EntityFrameworkCore.MySql provider.
/// </summary>
public class GapuraDbContext : DbContext
{
    public GapuraDbContext(DbContextOptions<GapuraDbContext> options)
        : base(options)
    {
    }

    // ----- DbSets -----
    public DbSet<AppAuth> AppsAuth { get; set; } = null!;
    public DbSet<ModelRegistry> ModelRegistry { get; set; } = null!;
    public DbSet<AuditLog> AuditLogs { get; set; } = null!;

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        base.OnModelCreating(modelBuilder);

        // ── Apps_Auth ──────────────────────────────────────────────────────
        modelBuilder.Entity<AppAuth>(entity =>
        {
            entity.ToTable("Apps_Auth");

            entity.HasKey(e => e.AppId);

            entity.Property(e => e.AppId)
                  .ValueGeneratedOnAdd();

            entity.Property(e => e.ProjectName)
                  .HasMaxLength(100)
                  .IsRequired();

            entity.Property(e => e.Username)
                  .HasMaxLength(50)
                  .IsRequired();

            entity.HasIndex(e => e.Username)
                  .IsUnique()
                  .HasDatabaseName("UQ_Apps_Auth_Username");

            entity.Property(e => e.PasswordHash)
                  .HasMaxLength(255)
                  .IsRequired();

            entity.Property(e => e.DailyTokenLimit)
                  .HasDefaultValue(100_000)
                  .IsRequired();
        });

        // ── Model_Registry ─────────────────────────────────────────────────
        modelBuilder.Entity<ModelRegistry>(entity =>
        {
            entity.ToTable("Model_Registry");

            entity.HasKey(e => e.ModelId);

            entity.Property(e => e.ModelId)
                  .ValueGeneratedOnAdd();

            entity.Property(e => e.ModelName)
                  .HasMaxLength(100)
                  .IsRequired();

            entity.Property(e => e.Provider)
                  .HasMaxLength(50)
                  .IsRequired();

            entity.Property(e => e.CostPer1kInput)
                  .HasColumnType("decimal(10,6)")
                  .HasDefaultValue(0.000000m)
                  .IsRequired();

            entity.Property(e => e.CostPer1kOutput)
                  .HasColumnType("decimal(10,6)")
                  .HasDefaultValue(0.000000m)
                  .IsRequired();

            entity.Property(e => e.IsLocalFallback)
                  .HasDefaultValue(false)
                  .IsRequired();
        });

        // ── Audit_Logs ─────────────────────────────────────────────────────
        modelBuilder.Entity<AuditLog>(entity =>
        {
            entity.ToTable("Audit_Logs");

            entity.HasKey(e => e.LogId);

            entity.Property(e => e.LogId)
                  .ValueGeneratedOnAdd();

            entity.Property(e => e.ModelUsed)
                  .HasMaxLength(100)
                  .IsRequired();

            entity.Property(e => e.OriginalPrompt)
                  .HasColumnType("text")
                  .IsRequired();

            entity.Property(e => e.ScrubbedPrompt)
                  .HasColumnType("text")
                  .IsRequired();

            entity.Property(e => e.ResponseText)
                  .HasColumnType("text");

            entity.Property(e => e.InputTokens)
                  .HasDefaultValue(0)
                  .IsRequired();

            entity.Property(e => e.OutputTokens)
                  .HasDefaultValue(0)
                  .IsRequired();

            entity.Property(e => e.CalculatedCost)
                  .HasColumnType("decimal(10,6)")
                  .HasDefaultValue(0.000000m)
                  .IsRequired();

            entity.Property(e => e.LatencyMs)
                  .HasColumnName("LatencyMS")
                  .HasDefaultValue(0)
                  .IsRequired();

            entity.Property(e => e.Timestamp)
                  .HasDefaultValueSql("CURRENT_TIMESTAMP")
                  .IsRequired();

            // Foreign Key: AppID → Apps_Auth
            entity.HasOne(e => e.App)
                  .WithMany(a => a.AuditLogs)
                  .HasForeignKey(e => e.AppId)
                  .OnDelete(DeleteBehavior.Restrict)
                  .HasConstraintName("FK_AuditLogs_AppsAuth");

            // Indexes
            entity.HasIndex(e => e.AppId)
                  .HasDatabaseName("IX_AuditLogs_AppID");

            entity.HasIndex(e => e.Timestamp)
                  .IsDescending()
                  .HasDatabaseName("IX_AuditLogs_Timestamp");
        });
    }
}
