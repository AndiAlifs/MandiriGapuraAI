using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace GapuraAI.API.Models;

/// <summary>
/// Immutable audit trail entry for every request processed by the GAPURA pipeline.
/// Maps to the 'Audit_Logs' MySQL table.
/// </summary>
[Table("Audit_Logs")]
public class AuditLog
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("LogID")]
    public long LogId { get; set; }

    [Required]
    [Column("AppID")]
    public int AppId { get; set; }

    [Required]
    [StringLength(100)]
    [Column("ModelUsed")]
    public string ModelUsed { get; set; } = string.Empty;

    [Required]
    [Column("OriginalPrompt", TypeName = "text")]
    public string OriginalPrompt { get; set; } = string.Empty;

    [Required]
    [Column("ScrubbedPrompt", TypeName = "text")]
    public string ScrubbedPrompt { get; set; } = string.Empty;

    [Column("ResponseText", TypeName = "text")]
    public string? ResponseText { get; set; }

    [Required]
    [Column("InputTokens")]
    public int InputTokens { get; set; }

    [Required]
    [Column("OutputTokens")]
    public int OutputTokens { get; set; }

    [Required]
    [Column("CalculatedCost", TypeName = "decimal(10,6)")]
    public decimal CalculatedCost { get; set; }

    [Required]
    [Column("LatencyMS")]
    public int LatencyMs { get; set; }

    [Column("Timestamp")]
    public DateTime Timestamp { get; set; } = DateTime.UtcNow;

    // Navigation property — each log belongs to one App
    [ForeignKey("AppID")]
    public AppAuth? App { get; set; }
}
