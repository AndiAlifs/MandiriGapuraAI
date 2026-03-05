using System.ComponentModel.DataAnnotations;
using System.ComponentModel.DataAnnotations.Schema;

namespace GapuraAI.API.Models;

/// <summary>
/// Represents an AI model in the catalog (cloud or local fallback).
/// Maps to the 'Model_Registry' MySQL table.
/// </summary>
[Table("Model_Registry")]
public class ModelRegistry
{
    [Key]
    [DatabaseGenerated(DatabaseGeneratedOption.Identity)]
    [Column("ModelID")]
    public int ModelId { get; set; }

    [Required]
    [StringLength(100)]
    [Column("ModelName")]
    public string ModelName { get; set; } = string.Empty;

    [Required]
    [StringLength(50)]
    [Column("Provider")]
    public string Provider { get; set; } = string.Empty;

    [Required]
    [Column("CostPer1kInput", TypeName = "decimal(10,6)")]
    public decimal CostPer1kInput { get; set; }

    [Required]
    [Column("CostPer1kOutput", TypeName = "decimal(10,6)")]
    public decimal CostPer1kOutput { get; set; }

    [Required]
    [Column("IsLocalFallback")]
    public bool IsLocalFallback { get; set; }
}
